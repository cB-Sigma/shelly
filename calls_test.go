// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package shelly_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/cB-Sigma/shelly"
)

// stub is a Caller that records requests and answers from a table.
type stub struct {
	calls   []string
	params  []map[string]any
	answers map[string][]string // method → answers in turn
}

func (s *stub) RPC(_ context.Context, method string, params map[string]any) (json.RawMessage, error) {
	b, _ := json.Marshal(params)
	s.calls = append(s.calls, method+" "+string(b))
	s.params = append(s.params, params)
	a := s.answers[method]
	if len(a) == 0 {
		return json.RawMessage("null"), nil
	}
	s.answers[method] = a[1:]
	if strings.HasPrefix(a[0], "error:") {
		return nil, &shelly.RPCError{Code: -105, Message: a[0][6:]}
	}
	return json.RawMessage(a[0]), nil
}

func TestScriptCodeChunks(t *testing.T) {
	st := &stub{answers: map[string][]string{}}
	code := strings.Repeat("let x = 'é'; ", 30) // two-octet characters on the chunk edges
	if err := shelly.PutScriptCode(context.Background(), st, 3, code, 7); err != nil {
		t.Fatal(err)
	}
	var got strings.Builder
	for i, p := range st.params {
		part := p["code"].(string)
		if !utf8.ValidString(part) || len(part) > 7 {
			t.Fatalf("chunk %d %q", i, part)
		}
		if p["append"] != (i > 0) || p["id"] != 3 {
			t.Fatalf("chunk %d params %v", i, p)
		}
		got.WriteString(part)
	}
	if got.String() != code {
		t.Fatal("the chunks do not add up to the code")
	}
	// Empty code still replaces (one request, append false).
	st = &stub{answers: map[string][]string{}}
	if err := shelly.PutScriptCode(context.Background(), st, 1, "", 0); err != nil || len(st.calls) != 1 || st.params[0]["append"] != false {
		t.Fatalf("empty code: %v %v", err, st.calls)
	}
}

func TestScriptGetCodePages(t *testing.T) {
	st := &stub{answers: map[string][]string{"Script.GetCode": {`{"data":"abc","left":3}`, `{"data":"def","left":0}`}}}
	code, err := shelly.GetScriptCode(context.Background(), st, 1)
	if err != nil || code != "abcdef" {
		t.Fatalf("%q %v", code, err)
	}
	if st.params[1]["offset"] != 3 {
		t.Fatalf("second page offset %v", st.params[1])
	}
}

func TestSchedulesWebhooksKVS(t *testing.T) {
	ctx := context.Background()
	st := &stub{answers: map[string][]string{
		"Schedule.Create":       {`{"id":1,"rev":4}`},
		"Schedule.List":         {`{"jobs":[{"id":1,"enable":true,"timespec":"0 0 7 * * MON-FRI","calls":[{"method":"Switch.Set","params":{"id":0,"on":true}}]}],"rev":4}`},
		"Webhook.ListSupported": {`{"types":{"switch.on":{},"input.button_push":{"attrs":[]}}}`, `{"hook_types":["switch.off","switch.on"]}`},
		"Webhook.Create":        {`{"id":2,"rev":7}`},
		"KVS.Set":               {`{"etag":"e1","rev":9}`},
		"KVS.Get":               {`{"etag":"e1","value":{"a":1}}`},
		"KVS.GetMany":           {`{"items":{"b":{"etag":"e2","value":2},"a":{"etag":"e1","value":1}}}`, `{"items":[{"key":"a","etag":"e1","value":1}],"offset":0,"total":2}`, `{"items":[{"key":"b","etag":"e2","value":"x"}],"offset":1,"total":2}`},
		"KVS.List":              {`{"keys":{"a":{"etag":"e1"}},"rev":9}`},
		"Shelly.GetComponents":  {`{"components":[{"key":"boolean:200","status":{"value":true}}],"cfg_rev":3,"offset":0,"total":2}`, `{"components":[{"key":"number:200","status":{"value":4}}],"cfg_rev":3,"offset":1,"total":2}`},
		"Virtual.Add":           {`{"id":201}`},
		"Switch.Set":            {`{"was_on":true}`, "error:Argument 'id', value 9 not found!"},
		"Script.Create":         {`{"id":5}`},
		"Script.Start":          {`{"was_running":false}`},
		"Shelly.GetStatus":      {`{"switch:0":{"id":0,"output":true}}`},
		"Webhook.List":          {`{"hooks":[{"id":2,"cid":0,"enable":true,"event":"switch.on","urls":["http://192.0.2.1/x"]}],"rev":7}`},
		"Script.Eval":           {`{"result":"42"}`},
		"Schedule.DeleteAll":    {`{"rev":5}`},
		"Shelly.GetDeviceInfo":  {`{"id":"shellyplus1-a8032ab1f001","gen":2,"mac":"A8032AB1F001"}`},
		"Switch.GetStatus":      {`{"id":0,"output":false,"apower":0}`},
		"Cover.GetStatus":       {`{"id":0,"state":"stopped","current_pos":null}`},
		"Shelly.GetConfig":      {`{"sys":{"device":{"name":"Hall"}}}`},
		"Sys.SetConfig":         {`{"restart_required":true}`},
	}}
	id, rev, err := shelly.CreateSchedule(ctx, st, shelly.ScheduleJob{Enable: true, Timespec: "0 0 7 * * MON-FRI",
		Calls: []shelly.ScheduleCall{{Method: "Switch.Set", Params: map[string]any{"id": 0, "on": true}}}})
	if err != nil || id != 1 || rev != 4 {
		t.Fatal(id, rev, err)
	}
	if _, has := st.params[0]["id"]; has {
		t.Fatal("create sent an id")
	}
	jobs, _, err := shelly.ListSchedules(ctx, st)
	if err != nil || len(jobs) != 1 || jobs[0].Calls[0].Method != "Switch.Set" || jobs[0].Calls[0].Params["on"] != true {
		t.Fatal(jobs, err)
	}
	if _, err := shelly.UpdateSchedule(ctx, st, shelly.ScheduleJob{}); err == nil {
		t.Fatal("update without an id accepted")
	}
	if rev, err := shelly.DeleteAllSchedules(ctx, st); err != nil || rev != 5 {
		t.Fatal(rev, err)
	}
	for _, want := range [][]string{{"input.button_push", "switch.on"}, {"switch.off", "switch.on"}} {
		ev, err := shelly.WebhookEvents(ctx, st)
		if err != nil || !reflect.DeepEqual(ev, want) {
			t.Fatal(ev, err)
		}
	}
	if id, _, err := shelly.CreateWebhook(ctx, st, shelly.Webhook{Event: "switch.on", Enable: true}); err != nil || id != 2 {
		t.Fatal(id, err)
	}
	last := st.params[len(st.params)-1]
	if u, ok := last["urls"].([]string); !ok || u == nil {
		t.Fatalf("urls sent as %#v", last["urls"])
	}
	hooks, _, err := shelly.ListWebhooks(ctx, st)
	if err != nil || len(hooks) != 1 || hooks[0].URLs[0] != "http://192.0.2.1/x" {
		t.Fatal(hooks, err)
	}
	if etag, rev, err := shelly.KVSSet(ctx, st, "a", map[string]any{"a": 1}, ""); err != nil || etag != "e1" || rev != 9 {
		t.Fatal(etag, rev, err)
	}
	if it, err := shelly.KVSGet(ctx, st, "a"); err != nil || string(it.Value) != `{"a":1}` || it.Key != "a" {
		t.Fatal(it, err)
	}
	old, err := shelly.KVSGetMany(ctx, st, "")
	if err != nil || len(old) != 2 || old[0].Key != "a" || old[1].Key != "b" {
		t.Fatal(old, err)
	}
	paged, err := shelly.KVSGetMany(ctx, st, "*")
	if err != nil || len(paged) != 2 || string(paged[1].Value) != `"x"` {
		t.Fatal(paged, err)
	}
	if keys, rev, err := shelly.KVSList(ctx, st, ""); err != nil || keys["a"] != "e1" || rev != 9 {
		t.Fatal(keys, rev, err)
	}
	comps, rev, err := shelly.GetComponents(ctx, st, true)
	if err != nil || len(comps) != 2 || rev != 3 || comps[1].Key != "number:200" {
		t.Fatal(comps, rev, err)
	}
	if id, err := shelly.AddVirtual(ctx, st, "number", map[string]any{"name": "Setpoint"}, 0); err != nil || id != 201 {
		t.Fatal(id, err)
	}
	if was, err := shelly.SetSwitch(ctx, st, 0, false, 30); err != nil || !was {
		t.Fatal(was, err)
	}
	if p := st.params[len(st.params)-1]; p["toggle_after"] != 30.0 {
		t.Fatal(p)
	}
	_, err = shelly.SetSwitch(ctx, st, 9, true, 0)
	if !shelly.IsRPCError(err, -105) {
		t.Fatalf("error %v", err)
	}
	var rpcErr *shelly.RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatal("not an RPCError")
	}
	if id, err := shelly.CreateScript(ctx, st, "x"); err != nil || id != 5 {
		t.Fatal(id, err)
	}
	if was, err := shelly.StartScript(ctx, st, 5); err != nil || was {
		t.Fatal(was, err)
	}
	if r, err := shelly.EvalScript(ctx, st, 5, "6*7"); err != nil || r != "42" {
		t.Fatal(r, err)
	}
	s, err := shelly.GetStatus(ctx, st)
	if sw, ok := s.Switch(0); err != nil || !ok || !sw.Output {
		t.Fatal(s, err)
	}
	if i, err := shelly.GetDeviceInfo(ctx, st); err != nil || i.Generation() != 2 {
		t.Fatal(i, err)
	}
	if sw, err := shelly.GetSwitch(ctx, st, 0); err != nil || sw.APower == nil || *sw.APower != 0 {
		t.Fatal(sw, err)
	}
	if cv, err := shelly.GetCover(ctx, st, 0); err != nil || cv.CurrentPos != nil || cv.State != "stopped" {
		t.Fatal(cv, err)
	}
	if cfg, err := shelly.GetConfig(ctx, st); err != nil || cfg.Name() != "Hall" {
		t.Fatal(cfg, err)
	}
	if restart, err := shelly.SetConfig(ctx, st, "sys", map[string]any{"device": map[string]any{"name": "Hall"}}); err != nil || !restart {
		t.Fatal(restart, err)
	}
	if _, has := st.params[len(st.params)-1]["id"]; has {
		t.Fatal("sys config sent an id")
	}
	if err := shelly.MoveCover(ctx, st, 0, "sideways", 0); err == nil {
		t.Fatal("a bad move was accepted")
	}
	if err := shelly.SetLight(ctx, st, "rgbw", 0, shelly.LightSet{RGB: []int{1, 2}}); err == nil {
		t.Fatal("two colour values accepted")
	}
	on, br := true, 40.0
	if err := shelly.SetLight(ctx, st, "rgbw", 0, shelly.LightSet{On: &on, Brightness: &br, RGB: []int{255, 0, 0}}); err != nil {
		t.Fatal(err)
	}
	if c := st.calls[len(st.calls)-1]; c != `RGBW.Set {"brightness":40,"id":0,"on":true,"rgb":[255,0,0]}` {
		t.Fatal(c)
	}
}

func TestMethodPrefixNamespaces(t *testing.T) {
	for typ, want := range map[string]string{"switch": "Switch", "wifi": "WiFi", "kvs": "KVS", "em1data": "EM1Data", "pm1": "PM1",
		"boolean": "Boolean", "cct": "CCT", "sys": "Sys", "": ""} {
		if got := shelly.MethodPrefix(typ); got != want {
			t.Errorf("MethodPrefix(%q) = %q, want %q", typ, got, want)
		}
	}
}

func TestCallerOverMQTTAndBLE(t *testing.T) {
	// MQTTDevice puts MQTT.RPC behind Caller; an error answer keeps its code.
	pub := &answeringPub{}
	m := &shelly.MQTT{Src: "app", Pub: pub}
	pub.m = m
	dev := shelly.MQTTDevice{M: m, Root: "shellyplus1-a8032ab1f001"}
	_, err := shelly.SetSwitch(context.Background(), dev, 3, true, 0)
	if !shelly.IsRPCError(err, -105) {
		t.Fatalf("MQTT error %v", err)
	}
	if !strings.Contains(err.Error(), "RPC error -105") {
		t.Fatalf("message %q", err)
	}
}

// answeringPub answers each request on <root>/rpc with an RPC error, as
// a device would over the broker.
type answeringPub struct{ m *shelly.MQTT }

func (p *answeringPub) Publish(_ context.Context, topic string, payload []byte, _ bool) error {
	var req struct {
		ID  int    `json:"id"`
		Src string `json:"src"`
	}
	_ = json.Unmarshal(payload, &req)
	go p.m.Handle(req.Src+"/rpc", []byte(fmt.Sprintf(`{"id":%d,"src":"dev","dst":"%s","error":{"code":-105,"message":"not found"}}`, req.ID, req.Src)))
	return nil
}
