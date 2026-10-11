// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package shelly_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cB-Sigma/shelly"
)

func fakeClient(base, password string) *shelly.Client {
	return &shelly.Client{Base: base, Password: password, HTTP: &http.Client{Timeout: 3 * time.Second}}
}

func TestGen1AgainstFake(t *testing.T) {
	d := &fakeDevice{gen: 1, password: "pw"}
	srv := d.start()
	defer srv.Close()
	info, err := fakeClient(srv.URL, "").Shelly(testCtx(t))
	if err != nil {
		t.Fatal(err)
	}
	if info.Generation() != 1 || info.ModelName() != "SHSW-1" || info.DefaultID() != "shelly-98cdac1ff001" || !info.Auth || !strings.HasSuffix(info.Firmware(), "v1.14.0-gcb84623") {
		t.Errorf("info = %+v", info)
	}
	if _, err := fakeClient(srv.URL, "").Get1(testCtx(t), "/status"); !errors.Is(err, shelly.ErrAuth) {
		t.Errorf("no password: %v", err)
	}
	if _, err := fakeClient(srv.URL, "wrong").Get1(testCtx(t), "/status"); !errors.Is(err, shelly.ErrAuth) {
		t.Errorf("wrong password: %v", err)
	}
	c := fakeClient(srv.URL, "pw")
	if _, err := c.Get1(testCtx(t), shelly.Gen1Turn("relay", 0, "on")); err != nil {
		t.Fatal(err)
	}
	st, err := c.Get1(testCtx(t), "/status")
	if err != nil {
		t.Fatal(err)
	}
	if relays, _ := st["relays"].([]any); len(relays) != 1 || relays[0].(map[string]any)["ison"] != true {
		t.Errorf("status = %v", st)
	}
	if _, err := c.Get1(testCtx(t), "/nothing"); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("unknown path: %v", err)
	}
	if d.refusals != 1 {
		t.Errorf("%d refusals, want 1 (the wrong password)", d.refusals)
	}
}

func TestGen2DigestAgainstFake(t *testing.T) {
	d := &fakeDevice{gen: 2, password: "secret"}
	srv := d.start()
	defer srv.Close()
	info, err := fakeClient(srv.URL, "").Shelly(testCtx(t))
	if err != nil || info.Generation() != 2 || info.DefaultID() != "shellyplus1-a8032ab1f001" || !info.AuthEn || info.Firmware() != "1.4.4" || info.App != "Plus1" {
		t.Fatalf("info = %+v %v", info, err)
	}
	if _, err := fakeClient(srv.URL, "").RPC(testCtx(t), "Shelly.GetStatus", nil); !errors.Is(err, shelly.ErrAuth) {
		t.Errorf("no password: %v", err)
	}
	if _, err := fakeClient(srv.URL, "nope").RPC(testCtx(t), "Shelly.GetStatus", nil); !errors.Is(err, shelly.ErrAuth) {
		t.Errorf("wrong password: %v", err)
	}
	c := fakeClient(srv.URL, "secret")
	// The first call answers the challenge; later ones reuse it with the
	// nonce count going up (the fake refuses a count that does not).
	for i := 0; i < 3; i++ {
		if _, err := c.RPC(testCtx(t), "Switch.Set", map[string]any{"id": 0, "on": i%2 == 0}); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := c.RPC(testCtx(t), "Shelly.GetStatus", nil)
	if err != nil {
		t.Fatal(err)
	}
	var st map[string]map[string]any
	_ = json.Unmarshal(raw, &st)
	if st["switch:0"]["output"] != true {
		t.Errorf("status = %s", raw)
	}
	_, err = c.RPC(testCtx(t), "Switch.Set", map[string]any{"id": 7, "on": true})
	var re *shelly.RPCError
	if !errors.As(err, &re) || re.Code != -105 {
		t.Errorf("rpc error = %v", err)
	}
	// No password: one challenge; a wrong one: two (the answer is
	// refused); the right one: one, answered for every later call.
	if d.nonces != 4 {
		t.Errorf("%d challenges, want 4", d.nonces)
	}
	if d.refusals != 1 {
		t.Errorf("%d refusals, want 1 (the wrong password)", d.refusals)
	}
	// A new nonce (the device restarted) is answered once more.
	d.mu.Lock()
	d.nonce = "restarted"
	d.mu.Unlock()
	if _, err := c.RPC(testCtx(t), "Shelly.GetStatus", nil); err != nil {
		t.Errorf("after a new nonce: %v", err)
	}
}

func TestCoIoTCodec(t *testing.T) {
	enc := shelly.EncodeCoIoT("SHSW-25", "A4CF12F45B2E", 300, []byte(`{"G":[]}`))
	m, err := shelly.ParseCoAP(enc)
	if err != nil || m.Serial != 300 || m.Code != 30 || m.Path != "/cit/s" || m.DevType != "SHSW-25" || m.DevID != "A4CF12F45B2E" || m.Version != "2" || string(m.Payload) != `{"G":[]}` {
		t.Errorf("round trip %+v %v", m, err)
	}
	for _, bad := range [][]byte{nil, {0x80, 0, 0, 0}, {0x5f, 30, 0, 1}, {0x50, 30, 0, 1, 0xd1}} {
		if _, err := shelly.ParseCoAP(bad); err == nil {
			t.Errorf("% x parsed", bad)
		}
	}
	desc := shelly.CitDesc{
		Blk: []shelly.CitBlock{{I: 1, D: "relay_0"}, {I: 2, D: "relay_1"}},
		Sen: []shelly.CitSensor{{I: 1101, T: "S", D: "output", L: 1}, {I: 1201, T: "S", D: "output", L: 2}, {I: 3101, T: "T", D: "temperature", U: "C", L: 9}},
	}
	vals := map[string]any{}
	for _, v := range desc.Decode([]byte(`{"G":[[0,1101,1],[0,1201,0],[0,3101,22.5],[0,9999,7]]}`)) {
		k := v.Sensor.D
		if v.HasIndex {
			k += "/" + string(rune('0'+v.Index))
		}
		vals[k] = v.Value
	}
	if len(vals) != 3 || vals["output/0"] != 1.0 || vals["output/1"] != 0.0 || vals["temperature"] != 22.5 {
		t.Errorf("values = %v", vals)
	}
	if shelly.Gen1Event("ss") != "double_push" || shelly.Gen1Event("X") != "x" {
		t.Error("Gen1Event")
	}
}

// A /cit/d sensor's L may be a list of block IDs (aioshelly reads both
// forms); such a description used to fail to decode as a whole.
func TestCitSensorBlockList(t *testing.T) {
	var d shelly.CitDesc
	err := json.Unmarshal([]byte(`{"blk":[{"I":1,"D":"relay_0"},{"I":2,"D":"relay_1"}],
		"sen":[{"I":9101,"T":"S","D":"mode","R":"color/white","L":[1,2]},{"I":1101,"T":"S","D":"output","R":"0/1","L":2}]}`), &d)
	if err != nil {
		t.Fatal(err)
	}
	if s := d.Sen[0]; s.L != 1 || len(s.Blocks) != 2 || s.Blocks[1] != 2 || s.D != "mode" || s.I != 9101 {
		t.Errorf("list sensor = %+v", s)
	}
	if s := d.Sen[1]; s.L != 2 || s.Blocks != nil {
		t.Errorf("number sensor = %+v", s)
	}
	if n, ok := d.BlockIndex(d.Sen[1]); !ok || n != 1 {
		t.Errorf("block index = %d %v", n, ok)
	}
	if err := json.Unmarshal([]byte(`{"blk":[],"sen":[{"I":1,"L":"x"}]}`), &d); err == nil {
		t.Error("a string L was accepted")
	}
}

func TestMQTTHandle(t *testing.T) {
	m := &shelly.MQTT{Src: "gw", Timeout: time.Second}
	if f := m.Filters(); len(f) != 4 || f[0] != "shellies/#" || f[3] != "gw/rpc" {
		t.Errorf("filters = %v", f)
	}
	const g2 = "shellyplus1-a8032ab11002"
	n := m.Handle(g2+"/events/rpc", []byte(`{"src":"`+g2+`","method":"NotifyStatus","params":{"switch:0":{"output":false}}}`))
	if n.Kind != shelly.MQTTNotification || n.Src != g2 || n.Root != g2 || n.Method != "NotifyStatus" {
		t.Errorf("notification = %+v", n)
	}
	a := m.Handle("shellies/announce", []byte(`{"id":"shellyplug-s-98CDAC1F0A0A","model":"SHPLG-S","mac":"98CDAC1F0A0A","ip":"192.168.1.77"}`))
	if a.Kind != shelly.MQTTAnnounce || a.Announce.IP != "192.168.1.77" || a.Root != "shellies/shellyplug-s-98CDAC1F0A0A" {
		t.Errorf("announce = %+v", a)
	}
	r := m.Handle("shellies/shelly1-98CDAC1F1001/relay/0", []byte("on"))
	if r.Kind != shelly.MQTTGen1 || r.Device != "shelly1-98CDAC1F1001" || r.Report.Kind != shelly.TopicRelay || !r.Report.On {
		t.Errorf("relay = %+v", r)
	}
	if r := shelly.ParseGen1Report([]string{"light", "1", "status"}, `{"ison":true,"brightness":40}`); r.Kind != shelly.TopicLightStatus || r.Index != 1 || r.Light["brightness"] != 40.0 {
		t.Errorf("light status = %+v", r)
	}
	if r := shelly.ParseGen1Report([]string{"roller", "0", "pos"}, "-1"); r.Kind != shelly.TopicRollerPos || r.Num != -1 {
		t.Errorf("roller pos = %+v", r)
	}
	if m.Handle("elsewhere/x", nil).Kind != shelly.MQTTNone || shelly.MACOfID("shelly1-A4CF12F45B2E") != "a4cf12f45b2e" || shelly.MACOfID("x") != "" {
		t.Error("other topics")
	}
}

func TestWSHelpers(t *testing.T) {
	if shelly.WSURL("https://h:8443") != "wss://h:8443/rpc" || shelly.WSURL("http://h") != "ws://h/rpc" {
		t.Error("WSURL")
	}
	ev := shelly.Events(map[string]any{"ts": 1.5, "events": []any{map[string]any{"component": "input:0", "id": 0.0, "event": "single_push", "ts": 1.5}}})
	if len(ev) != 1 || ev[0].Component != "input:0" || ev[0].Name != "single_push" || ev[0].TS != 1.5 {
		t.Errorf("events = %+v", ev)
	}
}
