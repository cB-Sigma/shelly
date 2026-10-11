// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package shelly

import (
	"crypto/subtle"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Webhooks and action URLs: the device calls a URL when something
// happens (Gen2+ webhooks, Gen1 actions and sensor report URLs). The
// device sends an HTTP GET; Gen2+ devices first replace ${...} tokens in
// the URL (${ev.tC}, ${status["temperature:0"].tC}, ${config.sys.device.mac},
// ...), and Gen1 sensors append their readings and their id
// (?hum=…&temp=…&id=shellyht-…). HookURL builds URLs in a shape HookHandler
// reads back, and HookHandler is the receiving end.

// Hook is one call a device made to a webhook or action URL.
type Hook struct {
	Device string     // the device named in the path, or the id a Gen1 sensor sent
	Event  string     // the event named in the path, or the "event" parameter
	Fields url.Values // the other query (and form) parameters
	Remote netip.Addr // the address the call came from
	Method string     // the HTTP method (devices send GET)
	Path   string     // the request path
	Time   time.Time  // when it was received
}

// Float reads field name as a number (a device sends "null" for a
// missing value; that, an empty value or a token the device did not
// replace give false).
func (h Hook) Float(name string) (float64, bool) {
	v := h.Fields.Get(name)
	f, err := strconv.ParseFloat(v, 64)
	return f, err == nil
}

// Bool reads field name as true or false (also 1 and 0, on and off,
// open and close).
func (h Hook) Bool(name string) (bool, bool) {
	switch strings.ToLower(h.Fields.Get(name)) {
	case "true", "1", "on", "open", "yes":
		return true, true
	case "false", "0", "off", "close", "closed", "no":
		return false, true
	}
	return false, false
}

// HookURL builds a URL for a device to call: base, then the device and
// the event as path segments, then fields as query parameters. Values may
// hold Gen2+ URL tokens (such as "${ev.tC}"); the token characters are
// kept as they are so that the device can replace them. token, when not
// empty, is added as the "token" parameter for HookHandler.Token.
//
//	HookURL("http://192.168.1.10:8080/hooks", "shellyplus1-a8032ab1e001", "switch.on",
//		map[string]string{"apower": `${status["switch:0"].apower}`}, "s3cret")
func HookURL(base, device, event string, fields map[string]string, token string) string {
	u := strings.TrimRight(base, "/")
	for _, seg := range []string{device, event} {
		if seg != "" {
			u += "/" + url.PathEscape(seg)
		}
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sortStrings(keys)
	var q []string
	for _, k := range keys {
		q = append(q, url.QueryEscape(k)+"="+escapeKeepTokens(fields[k]))
	}
	if token != "" {
		q = append(q, "token="+url.QueryEscape(token))
	}
	if len(q) > 0 {
		u += "?" + strings.Join(q, "&")
	}
	return u
}

// escapeKeepTokens query-escapes v but leaves ${...} tokens whole.
func escapeKeepTokens(v string) string {
	var b strings.Builder
	for v != "" {
		i := strings.Index(v, "${")
		if i < 0 {
			b.WriteString(url.QueryEscape(v))
			break
		}
		j := strings.Index(v[i:], "}")
		if j < 0 {
			b.WriteString(url.QueryEscape(v))
			break
		}
		b.WriteString(url.QueryEscape(v[:i]))
		b.WriteString(v[i : i+j+1])
		v = v[i+j+1:]
	}
	return b.String()
}

// HookHandler receives webhook and action URL calls. Mount it under
// Prefix (for example with http.Handle(Prefix+"/", h)); a call to
// Prefix/<device>/<event>?... becomes a Hook with Device and Event set,
// and a call to Prefix?... (as a Gen1 sensor's report URL) a Hook whose
// Device is its "id" parameter.
//
// Devices cannot sign their calls: Token, when set, must arrive as the
// "token" parameter (put it in the URL with HookURL), and Allow, when set,
// decides which source addresses are accepted. Refused calls get 403 and
// are not handed on.
type HookHandler struct {
	Prefix  string                // path prefix the handler is mounted at, such as "/hooks"
	Token   string                // shared secret expected as ?token=; empty: none
	Allow   func(netip.Addr) bool // source address check; nil: any
	Handle  func(Hook)            // called for each accepted call, on the request's goroutine
	MaxBody int64                 // form bodies above this are refused (default 64 KiB)
}

// ServeHTTP implements http.Handler. It answers 200 with an empty body,
// which is all devices look at.
func (h *HookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	remote, _ := netip.ParseAddr(host)
	remote = remote.Unmap()
	if h.Allow != nil && !h.Allow(remote) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	max := h.MaxBody
	if max <= 0 {
		max = 64 << 10
	}
	q := r.URL.Query()
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		body, err := io.ReadAll(io.LimitReader(r.Body, max+1))
		if err != nil || int64(len(body)) > max {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		if strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
			if form, err := url.ParseQuery(string(body)); err == nil {
				for k, vs := range form {
					for _, v := range vs {
						q.Add(k, v)
					}
				}
			}
		}
	}
	if h.Token != "" {
		got := q.Get("token")
		if subtle.ConstantTimeCompare([]byte(got), []byte(h.Token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}
	q.Del("token")
	hook := Hook{Fields: q, Remote: remote, Method: r.Method, Path: r.URL.Path, Time: time.Now()}
	rest := strings.TrimPrefix(r.URL.Path, strings.TrimRight(h.Prefix, "/"))
	var segs []string
	for _, s := range strings.Split(rest, "/") {
		if s != "" {
			segs = append(segs, s)
		}
	}
	if len(segs) > 0 {
		hook.Device = segs[0]
	}
	if len(segs) > 1 {
		hook.Event = strings.Join(segs[1:], "/")
	}
	if hook.Device == "" {
		hook.Device = q.Get("id")
		if hook.Device == "" {
			hook.Device = q.Get("device")
		}
	}
	if hook.Event == "" {
		hook.Event = q.Get("event")
	}
	if h.Handle != nil {
		h.Handle(hook)
	}
	w.WriteHeader(http.StatusOK)
}

// Gen1 action names for the common Gen1 events, for Gen1SetAction.
const (
	Gen1OutputOn     = "out_on_url"
	Gen1OutputOff    = "out_off_url"
	Gen1ButtonOn     = "btn_on_url"
	Gen1ButtonOff    = "btn_off_url"
	Gen1ShortPush    = "shortpush_url"
	Gen1LongPush     = "longpush_url"
	Gen1RollerOpen   = "roller_open_url"
	Gen1RollerClose  = "roller_close_url"
	Gen1RollerStop   = "roller_stop_url"
	Gen1SensorReport = "report_url"
)

// HookWebhook returns a Gen2+ webhook that calls a HookHandler at base
// for event on component instance cid, carrying fields (values may be
// URL tokens) and token. Name it, add a Condition, and create it with
// CreateWebhook.
func HookWebhook(base, device, event string, cid int, fields map[string]string, token string) Webhook {
	return Webhook{CID: cid, Enable: true, Event: event, URLs: []string{HookURL(base, device, event, fields, token)}}
}
