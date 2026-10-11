// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package shelly_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeDevice is a minimal stand-in for one device, enough for this
// module's own tests and examples: GET /shelly, Gen1 GETs with basic
// authentication and POST /rpc with SHA-256 digest authentication, whose
// check is written here without the package's helpers. The full simulator
// and the tests that drive it are in github.com/cB-Sigma/shelly-sim.
type fakeDevice struct {
	gen      int
	password string

	mu       sync.Mutex
	nonce    string
	lastNC   int64
	nonces   int
	output   bool
	refusals int // requests refused after a credential was offered
}

const (
	fakeGen2ID  = "shellyplus1-a8032ab1f001"
	fakeGen2MAC = "A8032AB1F001"
	fakeGen1MAC = "98CDAC1FF001"
)

// start serves the device until the test (or example) ends.
func (d *fakeDevice) start() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(d.serve))
}

func fakeSHA(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func fakeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (d *fakeDevice) serve(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if r.URL.Path == "/shelly" {
		if d.gen >= 2 {
			fakeJSON(w, 200, map[string]any{"id": fakeGen2ID, "mac": fakeGen2MAC, "model": "SNSW-001X16EU", "gen": d.gen,
				"fw_id": "20241011-114455/1.4.4-g8ab4e4a", "ver": "1.4.4", "app": "Plus1", "auth_en": d.password != ""})
		} else {
			fakeJSON(w, 200, map[string]any{"type": "SHSW-1", "mac": fakeGen1MAC, "auth": d.password != "", "fw": "20230913-114008/v1.14.0-gcb84623"})
		}
		return
	}
	if d.gen >= 2 {
		d.serveRPC(w, r)
		return
	}
	if d.password != "" {
		u, p, ok := r.BasicAuth()
		if !ok || u != "admin" || p != d.password {
			if ok {
				d.refusals++
			}
			w.Header().Set("WWW-Authenticate", `Basic realm="`+fakeGen1MAC+`"`)
			http.Error(w, "401 Unauthorized", http.StatusUnauthorized)
			return
		}
	}
	switch r.URL.Path {
	case "/relay/0":
		switch r.URL.Query().Get("turn") {
		case "on":
			d.output = true
		case "off":
			d.output = false
		}
		fakeJSON(w, 200, map[string]any{"ison": d.output})
	case "/status":
		fakeJSON(w, 200, map[string]any{"relays": []any{map[string]any{"ison": d.output}}})
	default:
		http.NotFound(w, r)
	}
}

// digestOK checks an Authorization header against the current nonce, with
// a nonce count that must go up.
func (d *fakeDevice) digestOK(r *http.Request) bool {
	h := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(h, "Digest ") || d.nonce == "" {
		return false
	}
	p := map[string]string{}
	for _, part := range strings.Split(h[len("Digest "):], ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok {
			p[k] = strings.Trim(v, `"`)
		}
	}
	nc, err := strconv.ParseInt(p["nc"], 16, 64)
	if err != nil || nc <= d.lastNC || p["nonce"] != d.nonce || p["username"] != "admin" || p["realm"] != fakeGen2ID || p["qop"] != "auth" {
		return false
	}
	ha1 := fakeSHA("admin:" + fakeGen2ID + ":" + d.password)
	ha2 := fakeSHA(r.Method + ":" + p["uri"])
	if p["response"] != fakeSHA(ha1+":"+p["nonce"]+":"+p["nc"]+":"+p["cnonce"]+":auth:"+ha2) {
		return false
	}
	d.lastNC = nc
	return true
}

func (d *fakeDevice) serveRPC(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/rpc" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	var req struct {
		ID     any            `json:"id"`
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fakeJSON(w, 400, map[string]any{"id": nil, "error": map[string]any{"code": -32700, "message": "parse error"}})
		return
	}
	if d.password != "" && !d.digestOK(r) {
		if r.Header.Get("Authorization") != "" {
			d.refusals++
		}
		d.nonces++
		d.nonce, d.lastNC = fmt.Sprintf("%x", time.Now().UnixNano()+int64(d.nonces)), 0
		w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Digest qop="auth", realm="%s", nonce="%s", algorithm=SHA-256`, fakeGen2ID, d.nonce))
		fakeJSON(w, 401, map[string]any{"id": req.ID, "src": fakeGen2ID, "error": map[string]any{"code": 401, "message": "Unauthorized"}})
		return
	}
	answer := map[string]any{"id": req.ID, "src": fakeGen2ID}
	switch req.Method {
	case "Shelly.GetStatus":
		answer["result"] = map[string]any{"switch:0": map[string]any{"id": 0, "output": d.output}}
	case "Switch.Set":
		if id, _ := req.Params["id"].(float64); id != 0 {
			answer["error"] = map[string]any{"code": -105, "message": "Argument 'id', value " + strconv.Itoa(int(id)) + " not found!"}
			break
		}
		was := d.output
		d.output, _ = req.Params["on"].(bool)
		answer["result"] = map[string]any{"was_on": was}
	default:
		answer["error"] = map[string]any{"code": -114, "message": "Method " + req.Method + " failed: No handler!"}
	}
	fakeJSON(w, 200, answer)
}

func testCtx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return c
}
