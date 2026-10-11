// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package shelly

import (
	"context"
	"net/url"
)

// First-time setup: a new device, or one reset to factory settings, opens
// its own Wi-Fi hotspot, where it answers at DefaultAPAddress, and (Gen2+)
// advertises the BLE RPC service. Either way it is given the site's
// Wi-Fi: Gen1 with GET /settings/sta, Gen2+ with WiFi.SetConfig.

// DefaultAPAddress is a Shelly device's address on its own hotspot.
const DefaultAPAddress = "192.168.33.1"

// WiFiConfig is the WiFi.SetConfig params that make a Gen2+ device join
// a network as a station.
func WiFiConfig(ssid, password string) map[string]any {
	return map[string]any{"config": map[string]any{"sta": map[string]any{"ssid": ssid, "pass": password, "enable": true}}}
}

// SetWiFi tells the device to join a network: WiFi.SetConfig for gen 2
// and later, GET /settings/sta for Gen1.
func (c *Client) SetWiFi(ctx context.Context, gen int, ssid, password string) error {
	if gen >= 2 {
		_, err := c.RPC(ctx, "WiFi.SetConfig", WiFiConfig(ssid, password))
		return err
	}
	q := url.Values{"enabled": {"1"}, "ssid": {ssid}, "key": {password}}
	_, err := c.Get1(ctx, "/settings/sta?"+q.Encode())
	return err
}
