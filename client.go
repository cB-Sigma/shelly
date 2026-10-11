package shelly

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// User is the user name of Gen2+ authentication (fixed by the API).
const User = "admin"

// ErrAuth is wrapped when the device refuses the credentials.
var ErrAuth = errors.New("the device refused the password")

// Info is what GET /shelly tells without authentication: Gen1 carries
// type and auth, Gen2+ gen, id, model, app, ver and auth_en.
type Info struct {
	// Gen is the generation (2, 3, 4); Gen1 devices leave it out.
	Gen int `json:"gen"`
	// ID is the Gen2+ device id (shellyplus2pm-a8032ab12345).
	ID string `json:"id"`
	// Name is the name the owner gave the device.
	Name string `json:"name"`
	// MAC is the MAC address as 12 hexadecimal digits.
	MAC string `json:"mac"`
	// Model is the Gen2+ model code (SNSW-102P16EU).
	Model string `json:"model"`
	// App is the Gen2+ application name (Plus2PM).
	App string `json:"app"`
	// Ver is the Gen2+ firmware version.
	Ver string `json:"ver"`
	// AuthEn is true when a Gen2+ device has a login.
	AuthEn bool `json:"auth_en"`
	// Type is the Gen1 type code (SHSW-25).
	Type string `json:"type"`
	// Auth is true when a Gen1 device has a login.
	Auth bool `json:"auth"`
	// FW is the Gen1 firmware version.
	FW string `json:"fw"`
}

// Generation is 1 for Gen1 devices, else the gen the device reports.
func (i Info) Generation() int {
	if i.Gen >= 2 {
		return i.Gen
	}
	return 1
}

// Firmware is the version to show.
func (i Info) Firmware() string {
	if i.Ver != "" {
		return i.Ver
	}
	return i.FW
}

// ModelName is the model to show.
func (i Info) ModelName() string {
	if i.Model != "" {
		return i.Model
	}
	return i.Type
}

// DefaultID is a device's identifier: the Gen2+ id in lower case, or
// shelly-<mac> for Gen1 (which reports none).
func (i Info) DefaultID() string {
	if i.ID != "" {
		return strings.ToLower(i.ID)
	}
	return "shelly-" + strings.ToLower(i.MAC)
}

// Client talks HTTP to one device. It is safe for concurrent use.
type Client struct {
	// Base is http://host[:port] (BaseURL makes one from an address).
	Base string
	// Username is the Gen1 basic-authentication user (default admin);
	// Gen2+ devices always use User.
	Username string
	// Password is the device's login password; empty when it has none.
	Password string
	// HTTP sends the requests. The application sets its timeouts, proxy
	// and dialer here.
	HTTP *http.Client

	mu     sync.Mutex
	ch     *challenge // last digest challenge (Gen2+ HTTP)
	nc     int
	nextID int
}

type challenge struct {
	realm, nonce, qop, opaque, algorithm string
}

// Shelly reads GET /shelly.
func (c *Client) Shelly(ctx context.Context) (Info, error) {
	var i Info
	b, err := c.get(ctx, "/shelly", false)
	if err != nil {
		return i, err
	}
	if err := json.Unmarshal(b, &i); err != nil {
		return i, fmt.Errorf("/shelly: %w", err)
	}
	return i, nil
}

// Get1 is a Gen1 GET of path (with query) as JSON, with basic
// authentication when a password is set.
func (c *Client) Get1(ctx context.Context, pathQuery string) (map[string]any, error) {
	b, err := c.get(ctx, pathQuery, true)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", pathQuery, err)
	}
	return m, nil
}

// Get is a Gen1 GET of path (with query), returning the body as it came:
// with basic authentication when a password is set. GET /cit/d (the CoIoT
// description) is one use.
func (c *Client) Get(ctx context.Context, pathQuery string) ([]byte, error) {
	return c.get(ctx, pathQuery, true)
}

func (c *Client) get(ctx context.Context, pathQuery string, auth bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+pathQuery, nil)
	if err != nil {
		return nil, err
	}
	if auth && c.Password != "" {
		u := c.Username
		if u == "" {
			u = User
		}
		req.SetBasicAuth(u, c.Password)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("%w (%s)", ErrAuth, pathQuery)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d %s", pathQuery, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return b, nil
}

// RPCError is an error object of a JSON-RPC answer.
type RPCError struct {
	// Code is the error code (401 asks for authentication; Shelly's own
	// codes are negative).
	Code int `json:"code"`
	// Message is the device's text (for a 401 over WebSocket, the
	// authentication challenge as JSON).
	Message string `json:"message"`
}

// Error is "RPC error <code>: <message>".
func (e *RPCError) Error() string { return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message) }

// RPC calls a Gen2+ method over HTTP (POST /rpc), answering a digest
// challenge when the device has authentication on.
func (c *Client) RPC(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()
	body := map[string]any{"id": id, "method": method}
	if params != nil {
		body["params"] = params
	}
	b, _ := json.Marshal(body)
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/rpc", bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if a := c.authorization(http.MethodPost, "/rpc"); a != "" {
			req.Header.Set("Authorization", a)
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		rb, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			ch := parseChallenge(resp.Header.Get("WWW-Authenticate"))
			if ch == nil || c.Password == "" || attempt > 0 {
				return nil, fmt.Errorf("%w (%s)", ErrAuth, method)
			}
			c.mu.Lock()
			c.ch, c.nc = ch, 0
			c.mu.Unlock()
			continue
		}
		var out struct {
			Result json.RawMessage `json:"result"`
			Error  *RPCError       `json:"error"`
		}
		if err := json.Unmarshal(rb, &out); err != nil {
			return nil, fmt.Errorf("%s: HTTP %d: %s", method, resp.StatusCode, strings.TrimSpace(string(rb)))
		}
		if out.Error != nil {
			return nil, out.Error
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: HTTP %d", method, resp.StatusCode)
		}
		return out.Result, nil
	}
}

// authorization builds the Authorization header for the last challenge
// (RFC 7616 with qop auth; the nonce count goes up per request).
func (c *Client) authorization(method, uri string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ch == nil || c.Password == "" {
		return ""
	}
	c.nc++
	nc := fmt.Sprintf("%08x", c.nc)
	cnonce := randomHex(8)
	resp := DigestResponse(User, c.ch.realm, c.Password, method, uri, c.ch.nonce, nc, cnonce)
	h := fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", algorithm=SHA-256, response="%s", qop=auth, nc=%s, cnonce="%s"`,
		User, c.ch.realm, c.ch.nonce, uri, resp, nc, cnonce)
	if c.ch.opaque != "" {
		h += `, opaque="` + c.ch.opaque + `"`
	}
	return h
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// SHA256Hex is the hexadecimal SHA-256 of s.
func SHA256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// DigestResponse is the RFC 7616 response with SHA-256 and qop auth:
// H(H(user:realm:password):nonce:nc:cnonce:auth:H(method:uri)).
func DigestResponse(user, realm, password, method, uri, nonce, nc, cnonce string) string {
	ha1 := SHA256Hex(user + ":" + realm + ":" + password)
	ha2 := SHA256Hex(method + ":" + uri)
	return SHA256Hex(ha1 + ":" + nonce + ":" + nc + ":" + cnonce + ":auth:" + ha2)
}

// parseChallenge reads a Digest WWW-Authenticate header; nil when it is
// not a digest challenge.
func parseChallenge(h string) *challenge {
	h = strings.TrimSpace(h)
	if len(h) < 7 || !strings.EqualFold(h[:6], "Digest") {
		return nil
	}
	p := map[string]string{}
	rest := h[6:]
	for len(rest) > 0 {
		rest = strings.TrimLeft(rest, " ,")
		eq := strings.IndexByte(rest, '=')
		if eq < 0 {
			break
		}
		k := strings.ToLower(strings.TrimSpace(rest[:eq]))
		rest = rest[eq+1:]
		var v string
		if strings.HasPrefix(rest, `"`) {
			end := strings.IndexByte(rest[1:], '"')
			if end < 0 {
				break
			}
			v, rest = rest[1:end+1], rest[end+2:]
		} else {
			end := strings.IndexByte(rest, ',')
			if end < 0 {
				end = len(rest)
			}
			v, rest = strings.TrimSpace(rest[:end]), rest[end:]
		}
		p[k] = v
	}
	if p["nonce"] == "" {
		return nil
	}
	return &challenge{realm: p["realm"], nonce: p["nonce"], qop: p["qop"], opaque: p["opaque"], algorithm: p["algorithm"]}
}

// WSAuth builds the authentication object of a JSON-RPC request over
// WebSocket from a 401 challenge (the error message is the challenge as
// JSON). The second hash is over the fixed "dummy_method:dummy_uri". The
// nonce and nonce count are echoed in the form the device sent them (a
// number on older firmware); without a count in the challenge it starts
// at 1.
func WSAuth(challengeJSON, password string) (map[string]any, error) {
	d := json.NewDecoder(strings.NewReader(challengeJSON))
	d.UseNumber()
	var ch map[string]any
	if err := d.Decode(&ch); err != nil {
		return nil, fmt.Errorf("the authentication challenge is not JSON: %w", err)
	}
	realm, _ := ch["realm"].(string)
	if ch["nonce"] == nil || realm == "" {
		return nil, errors.New("the authentication challenge has no nonce or realm")
	}
	if alg, _ := ch["algorithm"].(string); alg != "" && !strings.EqualFold(alg, "SHA-256") {
		return nil, fmt.Errorf("authentication algorithm %s is not supported", alg)
	}
	nc := ch["nc"]
	if nc == nil {
		nc = json.Number("1")
	}
	cn, _ := rand.Int(rand.Reader, big.NewInt(1<<40))
	cnonce := json.Number(strconv.FormatInt(cn.Int64(), 10))
	ha1 := SHA256Hex(User + ":" + realm + ":" + password)
	ha2 := SHA256Hex("dummy_method:dummy_uri")
	resp := SHA256Hex(ha1 + ":" + text(ch["nonce"]) + ":" + text(nc) + ":" + text(cnonce) + ":auth:" + ha2)
	return map[string]any{"realm": realm, "username": User, "nonce": ch["nonce"], "cnonce": cnonce, "nc": nc, "response": resp, "algorithm": "SHA-256"}, nil
}

func text(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	}
	return fmt.Sprint(v)
}

// BaseURL turns an address (host, host:port, http://host:port or
// https://host:port) into the Base of a Client.
func BaseURL(addr string) (string, error) {
	a := strings.TrimSpace(addr)
	if a == "" {
		return "", errors.New("no address")
	}
	if !strings.Contains(a, "://") {
		a = "http://" + a
	}
	u, err := url.Parse(a)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || (u.Path != "" && u.Path != "/") || u.RawQuery != "" {
		return "", fmt.Errorf("%q is not a host, host:port or http address", addr)
	}
	return u.Scheme + "://" + u.Host, nil
}
