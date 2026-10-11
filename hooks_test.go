// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package shelly

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
)

func TestHookURL(t *testing.T) {
	u := HookURL("http://10.0.0.5:8080/hooks/", "shellyplus1-a8032ab1e001", "switch.on",
		map[string]string{"apower": `${status["switch:0"].apower}`, "note": "a b&c"}, "s3cret")
	want := `http://10.0.0.5:8080/hooks/shellyplus1-a8032ab1e001/switch.on?apower=${status["switch:0"].apower}&note=a+b%26c&token=s3cret`
	if u != want {
		t.Fatalf("HookURL =\n%s\nwant\n%s", u, want)
	}
	if got := HookURL("http://h/x", "", "", nil, ""); got != "http://h/x" {
		t.Errorf("bare = %s", got)
	}
	w := HookWebhook("http://h/x", "dev", "input.button_push", 1, nil, "")
	if w.CID != 1 || !w.Enable || w.Event != "input.button_push" || w.URLs[0] != "http://h/x/dev/input.button_push" {
		t.Errorf("HookWebhook = %+v", w)
	}
}

func TestHookHandler(t *testing.T) {
	var mu sync.Mutex
	var got []Hook
	h := &HookHandler{Prefix: "/hooks", Token: "s3cret", Handle: func(k Hook) {
		mu.Lock()
		got = append(got, k)
		mu.Unlock()
	}}
	mux := http.NewServeMux()
	mux.Handle("/hooks/", h)
	mux.Handle("/hooks", h)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	get := func(path string) int {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	// A Gen2+ webhook with tokens already replaced by the device.
	if c := get("/hooks/shellyplusht-a8032ab1e003/temperature.change?tC=21.5&tF=70.7&token=s3cret"); c != 200 {
		t.Fatalf("status %d", c)
	}
	// A Gen1 H&T report URL (the device appends its readings and id).
	if c := get("/hooks?token=s3cret&hum=60&temp=25.00&id=shellyht-AABBCC"); c != 200 {
		t.Fatalf("status %d", c)
	}
	// Wrong token: refused, not handed on.
	if c := get("/hooks/x/y?token=nope"); c != http.StatusForbidden {
		t.Fatalf("wrong token: status %d", c)
	}
	// A form body.
	resp, err := http.Post(srv.URL+"/hooks/dev1/custom?token=s3cret", "application/x-www-form-urlencoded", strings.NewReader("door=open"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("%d hooks", len(got))
	}
	if k := got[0]; k.Device != "shellyplusht-a8032ab1e003" || k.Event != "temperature.change" || k.Fields.Get("token") != "" {
		t.Errorf("hook 0 = %+v", k)
	} else if f, ok := k.Float("tC"); !ok || f != 21.5 {
		t.Errorf("tC = %v %v", f, ok)
	}
	if k := got[1]; k.Device != "shellyht-AABBCC" || k.Event != "" {
		t.Errorf("hook 1 = %+v", k)
	} else if f, ok := k.Float("hum"); !ok || f != 60 {
		t.Errorf("hum = %v", f)
	}
	if k := got[2]; k.Method != http.MethodPost {
		t.Errorf("hook 2 = %+v", k)
	} else if b, ok := k.Bool("door"); !ok || !b {
		t.Errorf("door = %v %v", b, ok)
	}
	if !got[0].Remote.IsLoopback() {
		t.Errorf("remote = %v", got[0].Remote)
	}
	if _, ok := got[0].Float("missing"); ok {
		t.Error("a missing field read as a number")
	}
}

func TestHookHandlerAllow(t *testing.T) {
	calls := 0
	h := &HookHandler{Allow: func(a netip.Addr) bool { return a == netip.MustParseAddr("192.0.2.1") }, Handle: func(Hook) { calls++ }}
	r := httptest.NewRequest("GET", "/dev/ev", nil)
	r.RemoteAddr = "127.0.0.1:5555"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden || calls != 0 {
		t.Fatalf("code %d calls %d", w.Code, calls)
	}
	r.RemoteAddr = "192.0.2.1:5555"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || calls != 1 {
		t.Fatalf("code %d calls %d", w.Code, calls)
	}
}
