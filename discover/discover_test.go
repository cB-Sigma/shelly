// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package discover_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"testing"
	"time"

	"github.com/cB-Sigma/shelly"
	"github.com/cB-Sigma/shelly/discover"
)

func device(t *testing.T, body string) (*httptest.Server, int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/shelly" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	_, p, _ := net.SplitHostPort(srv.Listener.Addr().String())
	n, _ := strconv.Atoi(p)
	return srv, n
}

const gen2Info = `{"id":"shellyplus1pm-a8032ab1f101","mac":"A8032AB1F101","model":"SNSW-001P16EU","gen":2,"ver":"1.4.4","app":"Plus1PM","auth_en":false}`
const gen1Info = `{"type":"SHSW-1","mac":"98CDAC1FF102","auth":false,"fw":"20230913-114008/v1.14.0-gcb84623"}`

func TestFromService(t *testing.T) {
	lo := []net.IP{net.IPv4(127, 0, 0, 1)}
	d, ok := discover.FromService(discover.Service{Instance: "ShellyPlus1PM-A8032AB1F101", Port: 80, IPs: lo, TXT: map[string]string{"gen": "2", "app": "Plus1PM", "ver": "1.4.4"}})
	if !ok || d.Gen != 2 || d.App != "Plus1PM" || d.Addr != "127.0.0.1:80" || d.ID != "shellyplus1pm-a8032ab1f101" {
		t.Fatalf("%+v", d)
	}
	if d, ok := discover.FromService(discover.Service{Instance: "shelly1-98CDAC1FF102", Host: "shelly1-98cdac1ff102.local", Port: 80}); !ok || d.Gen != 0 || d.Addr != "shelly1-98cdac1ff102.local:80" {
		t.Fatalf("gen1 %+v %v", d, ok)
	}
	if _, ok := discover.FromService(discover.Service{Instance: "Living room printer", Port: 80, IPs: lo}); ok {
		t.Fatal("a printer taken for a Shelly")
	}
	if _, ok := discover.FromService(discover.Service{Instance: "shelly1-x", Port: 0, IPs: lo}); ok {
		t.Fatal("no port accepted")
	}
}

func TestMDNSConfirmsWithGETShelly(t *testing.T) {
	_, p2 := device(t, gen2Info)
	_, p1 := device(t, gen1Info)
	_, notShelly := device(t, `{"hello":"world"}`)
	lo := []net.IP{net.IPv4(127, 0, 0, 1)}
	b := func(_ context.Context, svc string) ([]discover.Service, error) {
		switch svc {
		case discover.ServiceShelly:
			return []discover.Service{{Instance: "shellyplus1pm-a8032ab1f101", Port: p2, IPs: lo, TXT: map[string]string{"gen": "2", "app": "Plus1PM"}}}, nil
		case discover.ServiceHTTP:
			return []discover.Service{
				{Instance: "shellyplus1pm-a8032ab1f101", Port: p2, IPs: lo}, // the same device under _http._tcp
				{Instance: "shelly1-98CDAC1FF102", Port: p1, IPs: lo},
				{Instance: "shellyfake", Port: notShelly, IPs: lo},
			}, nil
		}
		return nil, nil
	}
	got, err := discover.MDNS(context.Background(), b, discover.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("%d devices: %+v", len(got), got)
	}
	byID := map[string]discover.Device{}
	for _, d := range got {
		byID[d.ID] = d
	}
	if d := byID["shellyplus1pm-a8032ab1f101"]; d.Gen != 2 || d.Model != "SNSW-001P16EU" || d.Via != "mdns" || d.Info == nil {
		t.Fatalf("gen2 %+v", d)
	}
	if d := byID["shelly-98cdac1ff102"]; d.Gen != 1 || d.Model != "SHSW-1" {
		t.Fatalf("gen1 %+v", d)
	}
	raw, err := discover.MDNS(context.Background(), b, discover.Options{NoProbe: true})
	if err != nil || len(raw) != 3 {
		t.Fatalf("without probing: %d %v", len(raw), err)
	}
	failing := func(context.Context, string) ([]discover.Service, error) { return nil, errors.New("no multicast") }
	if _, err := discover.MDNS(context.Background(), failing, discover.Options{}); err == nil {
		t.Fatal("browser errors were dropped")
	}
}

func TestScanLoopback(t *testing.T) {
	_, p := device(t, gen2Info)
	got, err := discover.Scan(context.Background(), netip.MustParsePrefix("127.0.0.0/30"), discover.Options{Port: p, HTTP: &http.Client{Timeout: time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Addr != "127.0.0.1:"+strconv.Itoa(p) || got[0].Via != "scan" {
		t.Fatalf("%+v", got)
	}
	if _, err := discover.Scan(context.Background(), netip.MustParsePrefix("10.0.0.0/8"), discover.Options{}); err == nil {
		t.Fatal("a /8 was accepted")
	}
}

func TestCoIoTCandidates(t *testing.T) {
	_, p := device(t, gen1Info)
	l, err := shelly.ListenCoIoT(&net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}, "")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if d, ok := discover.FromCoIoT(shelly.CoIoTMessage{DevType: "SHSW-1", DevID: "98CDAC1FF102"}, &net.UDPAddr{IP: net.IPv4(192, 0, 2, 7)}); !ok || d.Addr != "192.0.2.7:80" || d.Gen != 1 {
		t.Fatalf("%+v", d)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	found := make(chan discover.Device, 4)
	// The CoIoT sender is on 127.0.0.1, so the probe goes to port 80 there;
	// NoProbe keeps the candidate as heard, and Probe confirms it at the
	// test server's port.
	go func() {
		_ = discover.ListenCoIoT(ctx, l, discover.Options{NoProbe: true}, func(d discover.Device) { found <- d })
	}()
	c, err := net.Dial("udp", l.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	msg := shelly.EncodeCoIoT("SHSW-1", "98CDAC1FF102", 1, []byte(`{"G":[[0,1101,1]]}`))
	for i := 0; i < 3; i++ { // repeats are reported once
		_, _ = c.Write(msg)
	}
	select {
	case d := <-found:
		if d.ID != "98cdac1ff102" || d.Via != "coiot" {
			t.Fatalf("%+v", d)
		}
	case <-ctx.Done():
		t.Fatal("no device from CoIoT")
	}
	select {
	case d := <-found:
		t.Fatalf("reported twice: %+v", d)
	case <-time.After(200 * time.Millisecond):
	}
	d, err := discover.Probe(context.Background(), nil, "127.0.0.1:"+strconv.Itoa(p))
	if err != nil || d.ID != "shelly-98cdac1ff102" || d.Gen != 1 {
		t.Fatalf("%+v %v", d, err)
	}
}
