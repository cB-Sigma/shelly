// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package shelly_test

import (
	"testing"

	"github.com/cB-Sigma/shelly"
)

func f(v float64) *float64 { return &v }

func TestGen1Requests(t *testing.T) {
	for got, want := range map[string]string{
		shelly.Gen1Turn("relay", 1, "toggle"):    "/relay/1?turn=toggle",
		shelly.Gen1Level(0, 0, false, nil):       "/light/0?turn=off",
		shelly.Gen1Level(0, 42.6, false, f(1.5)): "/light/0?brightness=43&transition=1500&turn=on",
		shelly.Gen1Level(1, 25, true, nil):       "/color/1?gain=25&turn=on",
		shelly.Gen1Level(1, 0, true, f(0.2)):     "/color/1?transition=200&turn=off",
		shelly.Gen1Colour(0, 255, 0, 10, nil):    "/color/0?blue=10&green=0&red=255&turn=on",
		shelly.Gen1Colour(0, 1, 2, 3, f(70)):     "/color/0?blue=3&gain=70&green=2&red=1&turn=on",
		shelly.Gen1Roller(0, "open"):             "/roller/0?go=open",
		shelly.Gen1RollerPos(0, 39.5):            "/roller/0?go=to_pos&roller_pos=40",
	} {
		if got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	}
	for pq, want := range map[string][2]string{
		"/relay/0?turn=on":                  {"relay/0/command", "on"},
		"/light/1?turn=off":                 {"light/1/command", "off"},
		"/color/0?gain=25&turn=on":          {"color/0/set", `{"gain":25,"turn":"on"}`},
		"/roller/0?go=stop":                 {"roller/0/command", "stop"},
		"/roller/0?go=to_pos&roller_pos=40": {"roller/0/command/pos", "40"},
	} {
		topic, payload, err := shelly.Gen1MQTT(pq)
		if err != nil || topic != want[0] || payload != want[1] {
			t.Errorf("%s: %s %s %v", pq, topic, payload, err)
		}
	}
	if _, _, err := shelly.Gen1MQTT("/settings"); err == nil {
		t.Error("/settings has an MQTT form")
	}
	if typ, id, ok := shelly.SplitKey("switch:1"); typ != "switch" || id != 1 || !ok {
		t.Error("SplitKey switch:1")
	}
	if typ, _, ok := shelly.SplitKey("sys"); typ != "sys" || ok {
		t.Error("SplitKey sys")
	}
	for in, want := range map[string]string{"switch": "Switch", "light": "Light", "rgb": "RGB", "rgbw": "RGBW", "cct": "CCT", "cover": "Cover", "": ""} {
		if shelly.MethodPrefix(in) != want {
			t.Errorf("MethodPrefix(%q) = %q", in, shelly.MethodPrefix(in))
		}
	}
}

func TestBaseURL(t *testing.T) {
	for in, want := range map[string]string{
		"192.168.1.5": "http://192.168.1.5", "192.168.1.5:8080": "http://192.168.1.5:8080",
		"http://h:81/": "http://h:81", "https://h": "https://h",
	} {
		if got, err := shelly.BaseURL(in); err != nil || got != want {
			t.Errorf("BaseURL(%q) = %q %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "ftp://h", "http://h/x", "http://h/?a=1"} {
		if _, err := shelly.BaseURL(bad); err == nil {
			t.Errorf("BaseURL(%q) accepted", bad)
		}
	}
}
