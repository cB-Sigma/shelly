// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/shelly"
)

// The fixture sources, at fixed revisions (see doc.go).
const (
	aioshellyRaw = "https://raw.githubusercontent.com/home-assistant-libs/aioshelly/13.34.1/"
	haCoreRaw    = "https://raw.githubusercontent.com/home-assistant/core/e8c451ce3967c8a94a6bcb5996e9cf72661a826a/tests/components/shelly/fixtures/"
)

var aioshellyDumps = []string{
	"gen1_Shelly Air_SHAIR-1_20230913-113804-v1.14.0-gcb84623.json",
	"gen1_Shelly Gas_SHGS-1_20230913-114427-v1.14.0-gcb84623.json",
	"gen1_Shelly Plug S_SHPLG-S_20230913-113421-v1.14.0-gcb84623.json",
	"gen1_Shelly Vintage_SHVIN-1_20230913-111730-v1.14.0-gcb84623.json",
	"gen2_Shelly BLU Gateway_SNGW-BT01_20231219-133956-1.1.0-g34b5d4f.json",
	"gen2_Shelly BLU Gateway_SNGW-BT01_20240213-140411-1.2.0-gb1b9aa8.json",
	"gen2_Shelly Plus 1PM_SNSW-001P16EU_20231219-133934-1.1.0-g34b5d4f.json",
	"gen2_Shelly Plus Plug IT_SNPL-00110IT_20231219-133948-1.1.0-g34b5d4f.json",
	"gen2_Shelly Plus Plug S_SNPL-00112EU_20231219-134003-1.1.0-g34b5d4f.json",
	"gen2_Shelly Pro 3EM_SPEM-003CEBEU_20231219-134001-1.1.0-g34b5d4f.json",
	"gen2_Shelly Pro 3_SPSW-003XE16EU_20231219-133956-1.1.0-g34b5d4f.json",
	"gen2_Shelly Pro 4PM_SPSW-004PE16EU_20230912-082358-1.0.3-g6176478.json",
	"gen2_Shelly Wall Display_SAWD-0A1XX10EU1_20240115-105444-1.2.8-59063a9a.json",
}

var haDumps = []string{
	"2pm_gen3.json", "2pm_gen3_cover.json", "cury_gen4.json", "duo_bulb_gen3.json", "fk-06x_gen3_irrigation.json",
	"power_strip_gen4.json", "presence_gen4.json", "pro1cb_gen2.json", "pro_3em.json", "st1820_gen3.json",
	"st802_gen3.json", "wall_display_xl.json",
}

func need(t *testing.T) {
	t.Helper()
	if os.Getenv("SIGMA_SHELLY_INTEROP") != "1" {
		t.Skip("fetches recorded device payloads from GitHub; set SIGMA_SHELLY_INTEROP=1 to run")
	}
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return c
}

// fetch gets a fixture file; base is a raw URL prefix, name a path below
// it (each segment escaped).
func fetch(t *testing.T, base, name string) []byte {
	t.Helper()
	seg := strings.Split(name, "/")
	for i := range seg {
		seg[i] = url.PathEscape(seg[i])
	}
	req, _ := http.NewRequestWithContext(ctx(t), http.MethodGet, base+strings.Join(seg, "/"), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("%s: HTTP %d %v", name, resp.StatusCode, err)
	}
	return b
}

// dump is one device's recording: /shelly (or Shelly.GetDeviceInfo), and
// Gen1 /settings and /status or Gen2+ Shelly.GetConfig and GetStatus.
type dump struct {
	Shelly   map[string]any `json:"shelly"`
	Settings map[string]any `json:"settings"`
	Status   map[string]any `json:"status"`
	Config   map[string]any `json:"config"`
}

// replay answers as the recorded device did.
func replay(t *testing.T, d dump) *httptest.Server {
	t.Helper()
	out := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/shelly":
			out(w, d.Shelly)
		case "/settings":
			out(w, d.Settings)
		case "/status":
			out(w, d.Status)
		case "/rpc":
			var req struct {
				ID     any    `json:"id"`
				Method string `json:"method"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			ans := map[string]any{"id": req.ID, "src": d.Shelly["id"]}
			switch req.Method {
			case "Shelly.GetDeviceInfo":
				ans["result"] = d.Shelly
			case "Shelly.GetStatus":
				ans["result"] = d.Status
			case "Shelly.GetConfig":
				ans["result"] = d.Config
			case "Ws.GetConfig":
				if ws, ok := d.Config["ws"]; ok {
					ans["result"] = ws
				} else {
					ans["error"] = map[string]any{"code": -114, "message": "Method Ws.GetConfig failed: No handler!"}
				}
			default:
				ans["error"] = map[string]any{"code": -114, "message": "Method " + req.Method + " failed: No handler!"}
			}
			out(w, ans)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// checkInfo compares the client's Info with the recorded /shelly.
func checkInfo(t *testing.T, info shelly.Info, sh map[string]any) {
	t.Helper()
	gen, _ := sh["gen"].(float64)
	if gen >= 2 {
		if info.Generation() != int(gen) || info.ModelName() != str(sh["model"]) || info.Firmware() != str(sh["ver"]) ||
			info.DefaultID() != strings.ToLower(str(sh["id"])) || info.App != str(sh["app"]) || info.AuthEn != (sh["auth_en"] == true) {
			t.Errorf("gen%d info = %+v, recorded %v", int(gen), info, sh)
		}
		return
	}
	if info.Generation() != 1 || info.ModelName() != str(sh["type"]) || info.Firmware() != str(sh["fw"]) ||
		info.DefaultID() != "shelly-"+strings.ToLower(str(sh["mac"])) || info.Auth != (sh["auth"] == true) {
		t.Errorf("gen1 info = %+v, recorded %v", info, sh)
	}
}

func checkDump(t *testing.T, raw []byte) {
	t.Helper()
	var d dump
	if err := json.Unmarshal(raw, &d); err != nil || d.Shelly == nil || d.Status == nil {
		t.Fatalf("not a device dump: %v", err)
	}
	srv := replay(t, d)
	c := &shelly.Client{Base: srv.URL, HTTP: &http.Client{Timeout: 5 * time.Second}}
	info, err := c.Shelly(ctx(t))
	if err != nil {
		t.Fatal(err)
	}
	checkInfo(t, info, d.Shelly)
	if info.Generation() == 1 {
		st, err := c.Get1(ctx(t), "/status")
		if err != nil || st["mac"] != d.Status["mac"] {
			t.Errorf("/status: %v %v", err, st["mac"])
		}
		set, err := c.Get1(ctx(t), "/settings")
		if err != nil || set["name"] != d.Settings["name"] {
			t.Errorf("/settings: %v", err)
		}
		t.Logf("%s %s gen1: %d status keys", info.DefaultID(), info.ModelName(), len(st))
		return
	}
	// Gen2+: the RPC answers, every component key, the outbound WebSocket
	// setting.
	raw2, err := c.RPC(ctx(t), "Shelly.GetStatus", nil)
	if err != nil {
		t.Fatal(err)
	}
	var st map[string]json.RawMessage
	if err := json.Unmarshal(raw2, &st); err != nil || len(st) != len(d.Status) {
		t.Fatalf("status: %d keys, %v", len(st), err)
	}
	types := map[string]bool{}
	for key := range st {
		typ, id, ok := shelly.SplitKey(key)
		_, n, hasID := strings.Cut(key, ":")
		if ok != hasID || typ == "" || (ok && fmt.Sprint(id) != n) {
			t.Errorf("SplitKey(%q) = %q %d %v", key, typ, id, ok)
		}
		types[typ] = true
	}
	for _, typ := range []string{"switch", "cover", "light", "rgb", "rgbw", "cct"} {
		if types[typ] && shelly.MethodPrefix(typ) == "" {
			t.Errorf("no method prefix for %s", typ)
		}
	}
	if d.Config != nil {
		ws, err := c.OutboundWS(ctx(t))
		rec, has := d.Config["ws"].(map[string]any)
		switch {
		case !has:
			if err == nil {
				t.Error("Ws.GetConfig refused, but OutboundWS succeeded")
			}
		case err != nil:
			t.Errorf("OutboundWS: %v", err)
		case ws.Enable != (rec["enable"] == true) || ws.Server != str(rec["server"]):
			t.Errorf("OutboundWS = %+v, recorded %v", ws, rec)
		}
	}
	t.Logf("%s %s gen%d: %d components", info.DefaultID(), info.ModelName(), info.Generation(), len(st))
}

// Devices recorded by aioshelly's fixture tool.
func TestAioshellyDeviceDumps(t *testing.T) {
	need(t)
	for _, name := range aioshellyDumps {
		t.Run(name[:strings.Index(name, "_20")], func(t *testing.T) {
			checkDump(t, fetch(t, aioshellyRaw, "fixtures/"+name))
		})
	}
}

// Devices recorded for Home Assistant's Shelly integration tests.
func TestHomeAssistantDeviceDumps(t *testing.T) {
	need(t)
	for _, name := range haDumps {
		t.Run(strings.TrimSuffix(name, ".json"), func(t *testing.T) {
			checkDump(t, fetch(t, haCoreRaw, name))
		})
	}
}

// A Plus 2PM with authentication on: its device info, and the 401 frame it
// sent over WebSocket, whose challenge WSAuth answers. The expected
// response is computed here from the challenge's fields.
func TestRecordedAuthChallenge(t *testing.T) {
	need(t)
	var info shelly.Info
	if err := json.Unmarshal(fetch(t, aioshellyRaw, "tests/rpc_device/fixtures/shellyplus2pm/shelly.json"), &info); err != nil {
		t.Fatal(err)
	}
	if !info.AuthEn || info.Generation() != 2 || info.DefaultID() != "shellyplus2pm-aabbccddeeff" {
		t.Errorf("info = %+v", info)
	}
	var frame struct {
		Src   string `json:"src"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(fetch(t, aioshellyRaw, "tests/rpc_device/fixtures/shellyplus2pm/Cover.Close_auth_failure"), &frame); err != nil || frame.Error.Code != 401 {
		t.Fatalf("frame: %v %+v", err, frame)
	}
	a, err := shelly.WSAuth(frame.Error.Message, "secret")
	if err != nil {
		t.Fatal(err)
	}
	var ch map[string]any
	d := json.NewDecoder(strings.NewReader(frame.Error.Message))
	d.UseNumber()
	_ = d.Decode(&ch)
	h := func(s string) string { x := sha256.Sum256([]byte(s)); return hex.EncodeToString(x[:]) }
	realm := ch["realm"].(string)
	want := h(h("admin:"+realm+":secret") + ":" + fmt.Sprint(ch["nonce"]) + ":" + fmt.Sprint(ch["nc"]) + ":" + fmt.Sprint(a["cnonce"]) + ":auth:" + h("dummy_method:dummy_uri"))
	b, _ := json.Marshal(a)
	if a["response"] != want || a["realm"] != frame.Src || a["username"] != "admin" ||
		!strings.Contains(string(b), `"nonce":`+fmt.Sprint(ch["nonce"])) || !strings.Contains(string(b), `"nc":1`) {
		t.Errorf("auth = %s (challenge %s)", b, frame.Error.Message)
	}
}
