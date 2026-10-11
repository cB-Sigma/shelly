// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package shelly_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cB-Sigma/shelly"
)

// Ask a device what it is, then switch a Gen2+ output with JSON-RPC.
// (github.com/cB-Sigma/shelly-sim has simulated devices of every kind.)
func Example() {
	srv := (&fakeDevice{gen: 2, password: "secret"}).start() // stands in for a device on the LAN
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	base, _ := shelly.BaseURL(srv.URL)
	c := &shelly.Client{Base: base, Password: "secret", HTTP: &http.Client{Timeout: 5 * time.Second}}
	info, err := c.Shelly(ctx)
	if err != nil {
		panic(err)
	}
	fmt.Println(info.DefaultID(), "gen", info.Generation(), info.ModelName())
	if _, err := c.RPC(ctx, "Switch.Set", map[string]any{"id": 0, "on": true}); err != nil {
		panic(err)
	}
	raw, _ := c.RPC(ctx, "Shelly.GetStatus", nil)
	var st map[string]map[string]any
	_ = json.Unmarshal(raw, &st)
	fmt.Println("output:", st["switch:0"]["output"])
	// Output:
	// shellyplus1-a8032ab1f001 gen 2 SNSW-001X16EU
	// output: true
}

// Dim a Gen1 light. A Bulb or RGBW2 in colour mode takes the level as
// the gain of /color/n.
func ExampleGen1Level() {
	fmt.Println(shelly.Gen1Level(0, 40, false, nil))
	fmt.Println(shelly.Gen1Level(0, 40, true, nil))
	// Output:
	// /light/0?brightness=40&turn=on
	// /color/0?gain=40&turn=on
}

// Read a CoIoT status message (what Gen1 devices multicast on UDP 5683).
func ExampleParseCoAP() {
	b := shelly.EncodeCoIoT("SHSW-1", "98CDAC1F0005", 7, []byte(`{"G":[[0,1101,1]]}`))
	m, err := shelly.ParseCoAP(b)
	if err != nil {
		panic(err)
	}
	desc := shelly.CitDesc{Blk: []shelly.CitBlock{{I: 1, D: "relay_0"}}, Sen: []shelly.CitSensor{{I: 1101, T: "S", D: "output", L: 1}}}
	for _, v := range desc.Decode(m.Payload) {
		fmt.Println(m.DevType, m.DevID, m.Path, v.Sensor.D, v.Index, v.Value)
	}
	// Output:
	// SHSW-1 98CDAC1F0005 /cit/s output 0 1
}
