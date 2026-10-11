// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package shelly_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/cB-Sigma/shelly"
)

// A status in the shape Gen2+ devices send (a Plus 2PM with a cover, a
// Pro 3EM, a Plus H&T and a Plus I4's counter input mixed together; the
// values are made up).
const statusJSON = `{
 "switch:0": {"id":0,"source":"init","output":true,"apower":820.5,"voltage":231.2,"current":3.6,"freq":50,"pf":0.98,
  "aenergy":{"total":15234.5,"by_minute":[110.5,112.0,111.2],"minute_ts":1760000000},"temperature":{"tC":41.2,"tF":106.2}},
 "switch:1": {"id":1,"output":false},
 "cover:0": {"id":0,"source":"rpc","state":"opening","apower":95,"current_pos":40,"target_pos":100,"move_timeout":30,"pos_control":true,"last_direction":"open"},
 "light:0": {"id":0,"output":true,"brightness":55,"source":"http"},
 "rgbw:0": {"id":0,"output":true,"brightness":80,"rgb":[255,10,0],"white":30},
 "input:0": {"id":0,"state":null},
 "input:1": {"id":1,"counts":{"total":512,"xtotal":51.2,"by_minute":[1,2,3],"minute_ts":1760000000},"freq":0.5},
 "input:2": {"id":2,"percent":44.5},
 "em:0": {"id":0,"a_current":1.2,"a_voltage":230.1,"a_act_power":250.5,"a_aprt_power":270,"a_pf":0.93,"a_freq":50,
  "b_current":0.5,"b_voltage":231,"b_act_power":100,"c_current":0,"c_voltage":229.8,"c_act_power":0,
  "n_current":null,"total_current":1.7,"total_act_power":350.5,"total_aprt_power":380},
 "emdata:0": {"id":0,"a_total_act_energy":1000.5,"b_total_act_energy":10,"c_total_act_energy":0,"total_act":1010.5,"total_act_ret":2},
 "em1:0": {"id":0,"current":2.1,"voltage":230,"act_power":480,"aprt_power":483,"pf":0.99,"freq":50},
 "em1data:0": {"id":0,"total_act_energy":5000,"total_act_ret_energy":0},
 "pm1:0": {"id":0,"voltage":230.4,"current":0.4,"apower":90,"freq":50,"aenergy":{"total":42}},
 "temperature:0": {"id":0,"tC":29.4,"tF":84.9},
 "humidity:0": {"id":0,"rh":71},
 "devicepower:0": {"id":0,"battery":{"V":5.9,"percent":88},"external":{"present":false}},
 "sys": {"mac":"A8032AB10001","restart_required":false,"time":"12:00","unixtime":1760000000,"uptime":1000,"ram_size":260000,"ram_free":120000,
  "cfg_rev":12,"kvs_rev":3,"schedule_rev":2,"webhook_rev":1,"available_updates":{"stable":{"version":"1.5.0"}}},
 "wifi": {"sta_ip":"192.0.2.10","status":"got ip","ssid":"Site","rssi":-60},
 "boolean:200": {"value":true}
}`

func f64(p *float64) float64 {
	if p == nil {
		return -1
	}
	return *p
}

func TestTypedStatus(t *testing.T) {
	s, err := shelly.ParseStatus([]byte(statusJSON))
	if err != nil {
		t.Fatal(err)
	}
	sw, ok := s.Switch(0)
	if !ok || !sw.Output || f64(sw.APower) != 820.5 || sw.AEnergy.Total != 15234.5 || len(sw.AEnergy.ByMinute) != 3 || f64(sw.Temp.C) != 41.2 || sw.Source != "init" {
		t.Fatalf("switch:0 %+v", sw)
	}
	if sw1, ok := s.Switch(1); !ok || sw1.Output || sw1.APower != nil || sw1.AEnergy != nil {
		t.Fatalf("switch:1 (no meter) %+v", sw1)
	}
	if _, ok := s.Switch(7); ok {
		t.Fatal("switch:7 found")
	}
	cv, _ := s.Cover(0)
	if cv.State != "opening" || f64(cv.CurrentPos) != 40 || f64(cv.TargetPos) != 100 || !cv.PosControl || f64(cv.APower) != 95 {
		t.Fatalf("cover %+v", cv)
	}
	if l, ok := s.Light("light", 0); !ok || f64(l.Brightness) != 55 {
		t.Fatalf("light %+v", l)
	}
	if l, ok := s.Light("rgbw", 0); !ok || !reflect.DeepEqual(l.RGB, []int{255, 10, 0}) || f64(l.White) != 30 {
		t.Fatalf("rgbw %+v", l)
	}
	if in, _ := s.Input(0); in.State != nil {
		t.Fatalf("input:0 state %v", *in.State)
	}
	if in, _ := s.Input(1); in.Counts == nil || in.Counts.Total != 512 || f64(in.Counts.XTotal) != 51.2 || f64(in.Freq) != 0.5 {
		t.Fatalf("input:1 %+v", in)
	}
	if in, _ := s.Input(2); f64(in.Percent) != 44.5 {
		t.Fatalf("input:2 %+v", in)
	}
	em, ok := s.EM(0)
	if !ok || f64(em.A.ActPower) != 250.5 || f64(em.B.Voltage) != 231 || f64(em.C.ActPower) != 0 || f64(em.TotalActPower) != 350.5 || em.NCurrent != nil || f64(em.A.PF) != 0.93 {
		t.Fatalf("em %+v", em)
	}
	if d, _ := s.EMData(0); f64(d.TotalAct) != 1010.5 || f64(d.ATotalActEnergy) != 1000.5 {
		t.Fatalf("emdata %+v", d)
	}
	if e, _ := s.EM1(0); f64(e.ActPower) != 480 || f64(e.PF) != 0.99 {
		t.Fatalf("em1 %+v", e)
	}
	if e, _ := s.EM1Data(0); f64(e.TotalActEnergy) != 5000 {
		t.Fatalf("em1data %+v", e)
	}
	if p, _ := s.PM1(0); f64(p.APower) != 90 || p.AEnergy.Total != 42 {
		t.Fatalf("pm1 %+v", p)
	}
	if v, _ := s.Temperature(0); f64(v.C) != 29.4 {
		t.Fatalf("temperature %+v", v)
	}
	if v, _ := s.Humidity(0); f64(v.RH) != 71 {
		t.Fatalf("humidity %+v", v)
	}
	if v, _ := s.DevicePower(0); f64(v.Battery.Percent) != 88 || v.External == nil || v.External.Present {
		t.Fatalf("devicepower %+v", v)
	}
	sys, _ := s.Sys()
	if sys.MAC != "A8032AB10001" || sys.KVSRev != 3 || sys.AvailableUpdates["stable"].Version != "1.5.0" || sys.Uptime != 1000 {
		t.Fatalf("sys %+v", sys)
	}
	if w, _ := s.WiFi(); w.StaIP == nil || *w.StaIP != "192.0.2.10" || f64(w.RSSI) != -60 {
		t.Fatalf("wifi %+v", w)
	}
	if k := s.Keys("input"); !reflect.DeepEqual(k, []string{"input:0", "input:1", "input:2"}) {
		t.Fatal(k)
	}
	// An unknown component stays readable as raw JSON.
	var b struct {
		Value bool `json:"value"`
	}
	if !s.Decode("boolean:200", &b) || !b.Value {
		t.Fatal("boolean:200")
	}
}

func TestTypedConfig(t *testing.T) {
	c, err := shelly.ParseConfig([]byte(`{
	 "switch:0":{"id":0,"name":"Pump","in_mode":"follow","initial_state":"restore_last","auto_off":true,"auto_off_delay":60,"power_limit":2800},
	 "cover:0":{"id":0,"name":null,"in_mode":"dual","invert_directions":true,"maxtime_open":60},
	 "input:0":{"id":0,"name":null,"type":"button","invert":false},
	 "light:0":{"id":0,"name":"Lamp","default":{"brightness":50},"night_mode":{"enable":true,"brightness":10},"transition_duration":1.5},
	 "sys":{"device":{"name":"Kitchen","mac":"A8032AB10001","fw_id":"x","discoverable":true},"location":{"tz":"Asia/Kolkata","lat":28.6,"lon":77.2},"cfg_rev":12},
	 "wifi":{"ap":{"ssid":"ShellyPlus2PM-A8032AB10001","enable":false,"is_open":true},"sta":{"ssid":"Site","enable":true,"is_open":false,"ipv4mode":"dhcp"},"sta1":{"ssid":null,"enable":false}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if sw, _ := c.Switch(0); *sw.Name != "Pump" || sw.InMode != "follow" || !sw.AutoOff || sw.AutoOffDelay != 60 || f64(sw.PowerLimit) != 2800 {
		t.Fatalf("switch %+v", sw)
	}
	if cv, _ := c.Cover(0); cv.Name != nil || !cv.InvertDirection || f64(cv.MaxtimeOpen) != 60 {
		t.Fatalf("cover %+v", cv)
	}
	if in, _ := c.Input(0); in.Type != "button" {
		t.Fatalf("input %+v", in)
	}
	if l, _ := c.Light("light", 0); f64(l.Default.Brightness) != 50 || !l.NightMode.Enable || f64(l.TransitionDur) != 1.5 {
		t.Fatalf("light %+v", l)
	}
	if c.Name() != "Kitchen" {
		t.Fatal(c.Name())
	}
	if sys, _ := c.Sys(); *sys.Location.TZ != "Asia/Kolkata" || sys.CfgRev != 12 {
		t.Fatalf("sys %+v", sys)
	}
	if w, _ := c.WiFi(); *w.STA.SSID != "Site" || !w.STA.Enable || w.STA1.SSID != nil {
		t.Fatalf("wifi %+v", w)
	}
	if _, err := shelly.ParseConfig([]byte(`[`)); err == nil {
		t.Fatal("bad JSON accepted")
	}
}

func TestGen1SettingsAndActions(t *testing.T) {
	if p := shelly.Gen1Settings("relay", 0, url.Values{"default_state": {"last"}, "auto_off": {"30"}}); p != "/settings/relay/0?auto_off=30&default_state=last" {
		t.Fatal(p)
	}
	if p := shelly.Gen1Settings("", 0, url.Values{"name": {"Pump"}}); p != "/settings?name=Pump" {
		t.Fatal(p)
	}
	if p := shelly.Gen1Settings("mqtt", -1, nil); p != "/settings/mqtt" {
		t.Fatal(p)
	}
	if p := shelly.Gen1SetAction("out_on_url", 0, true, "http://192.0.2.1/on"); p != "/settings/actions?enabled=true&index=0&name=out_on_url&urls%5B%5D=http%3A%2F%2F192.0.2.1%2Fon" {
		t.Fatal(p)
	}
	if p := shelly.Gen1SetAction("out_off_url", 0, false); p != "/settings/actions?enabled=false&index=0&name=out_off_url&urls%5B%5D=" {
		t.Fatal(p)
	}
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RequestURI())
		switch r.URL.Path {
		case "/settings/actions":
			// One action with plain URLs (older firmware), one with url
			// objects and a time window (1.10 and later).
			_, _ = w.Write([]byte(`{"actions":{"out_on_url":[{"index":0,"enabled":true,"urls":["http://192.0.2.1/on"]}],
			  "btn_on_url":[{"index":0,"enabled":false,"urls":[{"url":"http://192.0.2.1/b","int":"0000-0000"}]}]}}`))
		case "/settings/relay/0":
			_, _ = w.Write([]byte(`{"name":null,"ison":false,"default_state":"last","auto_off":30}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := &shelly.Client{Base: srv.URL, HTTP: srv.Client()}
	acts, err := c.Gen1Actions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if a := acts["out_on_url"][0]; !a.Enabled || a.URLs[0].URL != "http://192.0.2.1/on" {
		t.Fatalf("%+v", a)
	}
	if a := acts["btn_on_url"][0]; a.Enabled || a.URLs[0] != (shelly.Gen1ActionURL{URL: "http://192.0.2.1/b", Int: "0000-0000"}) {
		t.Fatalf("%+v", a)
	}
	if n := shelly.Gen1ActionNames(acts); !reflect.DeepEqual(n, []string{"btn_on_url", "out_on_url"}) {
		t.Fatal(n)
	}
	m, err := c.Settings1(context.Background(), "relay", 0, url.Values{"auto_off": {"30"}})
	if err != nil || m["auto_off"] != 30.0 {
		t.Fatal(m, err)
	}
	if seen[1] != "/settings/relay/0?auto_off=30" {
		t.Fatal(seen)
	}
}
