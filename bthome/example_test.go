// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package bthome_test

import (
	"encoding/hex"
	"fmt"

	"github.com/cB-Sigma/shelly/bthome"
)

// The encrypted example of the format's description: the service data,
// the sender's address and its bind key give temperature and humidity.
func ExampleDecode() {
	sd, _ := hex.DecodeString("41e445f3c9962b332211006c7c4519")
	key, _ := hex.DecodeString("231d39c1d7cc1ab1aee224cd096db932")
	p, err := bthome.Decode(bthome.UUID, sd, "54:48:E6:8F:80:A5", key)
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, o := range p.Objects {
		fmt.Println(o.Info.Name, o.Value, o.Info.Unit)
	}
	// Output:
	// temperature 25.06 °C
	// humidity 50.55 %
}

// Building a door sensor's packet with a button press.
func ExampleBuilder() {
	var b bthome.Builder
	b.Trigger = true
	press, _ := bthome.ButtonEvent("press")
	sd, _ := b.Add(0x01, 90).Add(0x2D, 1).Button(press).ServiceData()
	fmt.Printf("%x\n", sd)
	// Output:
	// 44015a2d013a01
}
