// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package shelly

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cB-Sigma/shelly/bthome"
)

// recorder answers calls from a table and records them.
type recorder struct {
	calls   []string
	params  []map[string]any
	answers map[string]string
}

func (r *recorder) RPC(_ context.Context, method string, params map[string]any) (json.RawMessage, error) {
	r.calls = append(r.calls, method)
	r.params = append(r.params, params)
	return json.RawMessage(r.answers[method]), nil
}

func TestBTHomeCalls(t *testing.T) {
	r := &recorder{answers: map[string]string{
		"BTHome.AddDevice":              `{"added":"bthomedevice:201"}`,
		"BTHome.AddSensor":              `{"key":"bthomesensor:200"}`,
		"BTHome.GetObjectInfos":         `{"objects":[{"obj_id":2,"obj_name":"temperature","type":"sensor","unit":"° C"}],"offset":0,"count":1,"total":1}`,
		"BTHomeDevice.GetKnownObjects":  `{"id":200,"objects":[{"obj_id":5,"idx":3,"component":"bthomesensor:200"},{"obj_id":1,"idx":0,"component":null}]}`,
		"BTHome.DeleteDevice":           `null`,
		"BTHome.StartDeviceDiscovery":   `null`,
		"BTHome.ResetEncryptionCounter": `null`,
	}}
	ctx := context.Background()
	name := "Door"
	k, id, err := BTHomeAddDevice(ctx, r, BTHomeDeviceConfig{Addr: "3c:2e:f5:71:d5:2a", Name: &name, Key: "00112233445566778899aabbccddeeff"}, 0)
	if err != nil || k != "bthomedevice:201" || id != 201 {
		t.Fatalf("AddDevice = %q %d %v", k, id, err)
	}
	conf := r.params[0]["config"].(map[string]any)
	if conf["addr"] != "3c:2e:f5:71:d5:2a" || conf["key"] == nil || r.params[0]["id"] != nil {
		t.Errorf("AddDevice params %v", r.params[0])
	}
	k, id, err = BTHomeAddSensor(ctx, r, BTHomeSensorConfig{Addr: "3c:2e:f5:71:d5:2a", ObjID: 45, Idx: 0}, 200)
	if err != nil || k != "bthomesensor:200" || id != 200 || r.params[1]["id"] != 200 {
		t.Fatalf("AddSensor = %q %d %v %v", k, id, err, r.params[1])
	}
	infos, err := BTHomeObjectInfos(ctx, r, []int{2})
	if err != nil || len(infos) != 1 || infos[0].Name != "temperature" || infos[0].Type != "sensor" {
		t.Errorf("infos %+v %v", infos, err)
	}
	objs, err := BTHomeKnownObjects(ctx, r, 200)
	if err != nil || len(objs) != 2 || *objs[0].Component != "bthomesensor:200" || objs[1].Component != nil {
		t.Errorf("known %+v %v", objs, err)
	}
	if err := BTHomeDeleteDevice(ctx, r, 201); err != nil {
		t.Error(err)
	}
	if err := BTHomeStartDiscovery(ctx, r, 25); err != nil || r.params[len(r.params)-1]["duration"] != 25 {
		t.Error("discovery", err)
	}
	if err := BTHomeResetEncryptionCounter(ctx, r); err != nil {
		t.Error(err)
	}
	if MethodPrefix("bthomedevice") != "BTHomeDevice" {
		t.Errorf("prefix %s", MethodPrefix("bthomedevice"))
	}
}

func TestBTHomeViews(t *testing.T) {
	st, err := ParseStatus([]byte(`{"bthomedevice:200":{"id":200,"rssi":-58,"battery":100,"packet_id":173,"last_updated_ts":1776351991,"key":false,"paired":true,"rpc":false,"rsv":-1,"fw_ver":"v1.0.21"},
		"bthomesensor:200":{"id":200,"value":false,"last_updated_ts":1706593991.91},"bthomesensor:201":{"id":201,"value":21.4,"last_updated_ts":1},
		"smoke:0":{"id":0,"alarm":true,"mute":false},"illuminance:0":{"id":0,"lux":120,"illumination":"twilight"},"voltmeter:100":{"id":100,"voltage":3.3,"xvoltage":33}}`))
	if err != nil {
		t.Fatal(err)
	}
	d, ok := st.BTHomeDevice(200)
	if !ok || *d.RSSI != -58 || *d.Battery != 100 || d.FWVer != "v1.0.21" || !*d.Paired {
		t.Errorf("device %+v", d)
	}
	s0, _ := st.BTHomeSensor(200)
	if v, ok := s0.Float(); !ok || v != 0 {
		t.Errorf("bool sensor %v", s0.Value)
	}
	s1, _ := st.BTHomeSensor(201)
	if v, ok := s1.Float(); !ok || v != 21.4 {
		t.Errorf("number sensor %v", s1.Value)
	}
	if a, ok := st.Smoke(0); !ok || !a.Alarm {
		t.Errorf("smoke %+v", a)
	}
	if l, ok := st.Illuminance(0); !ok || *l.Lux != 120 || l.Illumination != "twilight" {
		t.Errorf("illuminance %+v", l)
	}
	if v, ok := st.Voltmeter(100); !ok || *v.XVoltage != 33 {
		t.Errorf("voltmeter %+v", v)
	}
	cfg, _ := ParseConfig([]byte(`{"bthomesensor:200":{"id":200,"addr":"3c:2e:f5:71:d5:2a","name":"Door status","meta":null,"obj_id":45,"idx":3}}`))
	if c, ok := cfg.BTHomeSensor(200); !ok || c.ObjID != 45 || c.Idx != 3 || *c.Name != "Door status" {
		t.Errorf("sensor config %+v", c)
	}
}

func TestBTHomeEvents(t *testing.T) {
	var p map[string]any
	_ = json.Unmarshal([]byte(`{"ts":1,"events":[
		{"component":"bthome","event":"device_discovered","device":{"addr":"3c:2e:f5:71:d5:2a","local_name":"SBBT-002C","rssi":-59,"encrypted":false,"shelly_mfdata":{"flags":25,"model_id":1,"mac":"3c:2e:f5:71:d5:2a"}},"ts":1706593991.91},
		{"component":"bthomedevice:202","id":202,"event":"single_push","idx":1,"channel":0,"ts":1731931521.19,"sensors":{"1":[{"id":203,"value":100,"last_updated_ts":1747989100}]}}]}`), &p)
	evs := Events(p)
	d, ok := ParseBTHomeDiscovered(evs[0])
	if !ok || d.LocalName != "SBBT-002C" || d.RSSI != -59 || d.Shelly == nil || d.Shelly.ModelID != 1 {
		t.Errorf("discovered %+v", d)
	}
	e, ok := ParseBTHomeDeviceEvent(evs[1])
	if !ok || e.Event != "single_push" || e.Idx != 1 || e.Channel != 0 || len(e.Sensors["1"]) != 1 || e.Sensors["1"][0].ID != 203 {
		t.Errorf("device event %+v", e)
	}
	if _, ok := ParseBTHomeDeviceEvent(evs[0]); ok {
		t.Error("a bthome event read as a device event")
	}
}

func TestBLEScanResult(t *testing.T) {
	var b bthome.Builder
	sd, _ := b.Add(0x01, 90).Add(0x2D, 1).ServiceData()
	adv := bthome.Advertising(sd, "SBDW-002C")
	// The manufacturer data structure: length, 0xFF, company id, blocks.
	blocks := []byte{0x01, 0x19, 0x00, 0x0B, 0x02, 0x00, 0x0A, 0x2A, 0xD5, 0x71, 0xF5, 0x2E, 0x3C}
	ad := append([]byte{byte(3 + len(blocks)), 0xFF, 0xA9, 0x0B}, blocks...)
	data := ScanResultData([]BLEAdvert{{Addr: "3C:2E:F5:71:D5:2A", RSSI: -60, Data: append(adv, ad...)}})
	if !strings.HasPrefix(string(data), `[2,[["3c:2e:f5:71:d5:2a",-60,"`) {
		t.Fatalf("data %s", data)
	}
	got, err := ParseBLEScanResult(data)
	if err != nil || len(got) != 1 || got[0].Addr != "3C:2E:F5:71:D5:2A" || got[0].RSSI != -60 {
		t.Fatalf("parse %+v %v", got, err)
	}
	pk, err := got[0].BTHome(nil)
	if err != nil {
		t.Fatal(err)
	}
	if o, ok := pk.Find(0x2D, 0); !ok || !o.Bool() {
		t.Errorf("window %+v", o)
	}
	m, ok := got[0].Shelly()
	if !ok || m.Flags != 0x19 || m.ModelID != 2 || m.MAC != "3C:2E:F5:71:D5:2A" {
		t.Errorf("mfdata %+v %v", m, ok)
	}
	// Version 1, as older scripts send it, with a scan response.
	v1 := `[1,"aa:bb:cc:dd:ee:ff",-70,"` + base64.StdEncoding.EncodeToString(adv) + `","` + base64.StdEncoding.EncodeToString([]byte{3, 9, 'h', 'i'}) + `"]`
	got, err = ParseBLEScanResult(json.RawMessage(v1))
	if err != nil || len(got) != 1 || got[0].RSSI != -70 {
		t.Fatalf("v1 %+v %v", got, err)
	}
	ads, _ := bthome.ParseAD(got[0].Data)
	if bthome.LocalName(ads) != "SBDW-002C" {
		t.Errorf("name %q", bthome.LocalName(ads))
	}
	ev := Event{Name: BLEScanResultEvent, Fields: map[string]any{}}
	var dv any
	_ = json.Unmarshal(data, &dv)
	ev.Fields["data"] = dv
	if a, ok, err := ParseBLEScanEvent(ev); !ok || err != nil || len(a) != 1 {
		t.Errorf("event %v %v %v", a, ok, err)
	}
	if _, err := ParseBLEScanResult(json.RawMessage(`[3,[]]`)); err == nil {
		t.Error("version 3 accepted")
	}
	code := BLERelayScript(true)
	if !strings.Contains(code, BLEScanResultEvent) || !strings.Contains(code, "active: true") {
		t.Errorf("script %s", code)
	}
}
