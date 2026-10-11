// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package shelly

import (
	"encoding/json"
	"strings"
	"testing"
)

// These two tests moved here from SigmaM2M's Shelly adapter tests with
// the code they test; their Covers lines are SigmaM2M requirement IDs.

// Covers: SIG-IPE-4396
func TestShellyDigest(t *testing.T) {
	// RFC 7616 section 3.9.1, the SHA-256 example.
	got := DigestResponse("Mufasa", "http-auth@example.org", "Circle of Life", "GET", "/dir/index.html",
		"7ypf/xlj9XXwfDPEoM4URrv/xwf94BcCAzFZH4GiTo0v", "00000001", "f2/wE4q74E6zIJEtWaHKaf5wv/H5QzzpXusqGemxURZJ")
	if got != "753927fa0e85d155564e2e272a28d1802ca10daf4496794697cf8db5856cb6c1" {
		t.Errorf("digest = %s", got)
	}
	ch := parseChallenge(`Digest qop="auth", realm="shellyplus1-a8032ab1b2c3", nonce="60dc59c6", algorithm=SHA-256`)
	if ch == nil || ch.realm != "shellyplus1-a8032ab1b2c3" || ch.nonce != "60dc59c6" || ch.qop != "auth" || !strings.EqualFold(ch.algorithm, "SHA-256") {
		t.Errorf("challenge = %+v", ch)
	}
	if parseChallenge(`Basic realm="x"`) != nil {
		t.Error("a basic challenge was read as digest")
	}
	// The header counts nonces up and names the fixed user.
	c := &Client{Password: "pw"}
	c.ch = ch
	h1, h2 := c.authorization("POST", "/rpc"), c.authorization("POST", "/rpc")
	if !strings.Contains(h1, "nc=00000001") || !strings.Contains(h2, "nc=00000002") || !strings.Contains(h1, `username="admin"`) || !strings.Contains(h1, `uri="/rpc"`) {
		t.Errorf("headers %s / %s", h1, h2)
	}
}

// Covers: SIG-IPE-4397
func TestShellyWebSocketAuthObject(t *testing.T) {
	a, err := WSAuth(`{"auth_type":"digest","nonce":1625038762,"nc":1,"realm":"shellypro4pm-f008d1d8b8b8","algorithm":"SHA-256"}`, "pw")
	if err != nil {
		t.Fatal(err)
	}
	ha1 := SHA256Hex("admin:shellypro4pm-f008d1d8b8b8:pw")
	ha2 := SHA256Hex("dummy_method:dummy_uri")
	want := SHA256Hex(ha1 + ":1625038762:1:" + text(a["cnonce"]) + ":auth:" + ha2)
	b, _ := json.Marshal(a)
	if a["response"] != want || a["username"] != "admin" || a["realm"] != "shellypro4pm-f008d1d8b8b8" || a["algorithm"] != "SHA-256" ||
		!strings.Contains(string(b), `"nonce":1625038762`) || !strings.Contains(string(b), `"nc":1`) {
		t.Errorf("auth = %s", b)
	}
	// Newer firmware: a string nonce and no count.
	a, _ = WSAuth(`{"auth_type":"digest","nonce":"abc=","realm":"r","algorithm":"SHA-256"}`, "pw")
	if a["nonce"] != "abc=" || text(a["nc"]) != "1" {
		t.Errorf("auth = %v", a)
	}
	if _, err := WSAuth(`{"nonce":1,"realm":"r","algorithm":"MD5"}`, "pw"); err == nil {
		t.Error("MD5 accepted")
	}
	if _, err := WSAuth(`not json`, "pw"); err == nil {
		t.Error("garbage accepted")
	}
}
