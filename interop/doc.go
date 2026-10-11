// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Package interop holds tests only: this module's client reads payloads
// recorded from real Shelly devices, which other projects publish as test
// fixtures (Apache-2.0):
//
//   - aioshelly (github.com/home-assistant-libs/aioshelly) at tag 13.34.1:
//     the device dumps in fixtures/ (four Gen1 devices with /shelly,
//     /settings and /status; nine Gen2 devices with the device info,
//     Shelly.GetConfig and Shelly.GetStatus) and, from
//     tests/rpc_device/fixtures/shellyplus2pm, a device info with
//     authentication on and a WebSocket 401 frame with its challenge.
//   - Home Assistant core (github.com/home-assistant/core) at commit
//     e8c451ce3967c8a94a6bcb5996e9cf72661a826a: the device dumps in
//     tests/components/shelly/fixtures (Gen2, Gen3 and Gen4 devices).
//
// bthome_test.go runs bthome-ble 3.24.0 (MIT; github.com/Bluetooth-Devices/bthome-ble,
// the parser Home Assistant uses) in a python:3.12.10-slim container:
// package bthome builds a packet for every object id it knows (plain and
// encrypted), bthome-ble decodes each with testdata/bthome_peer.py, and
// the values must equal those package bthome decodes. It needs Docker.
//
// The files are fetched at test time from raw.githubusercontent.com at
// those fixed revisions; none is kept in this repository. A small server
// in the test answers GET /shelly, the Gen1 GETs and POST /rpc with the
// recorded payloads, so the client's HTTP, JSON-RPC and decoding run on
// what real devices sent; the serving itself is ours. The tests run only
// when SIGMA_SHELLY_INTEROP=1 and need network access:
//
//	cd interop && SIGMA_SHELLY_INTEROP=1 GOWORK=off go test -v ./...
package interop
