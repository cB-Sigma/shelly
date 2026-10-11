// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Package shelly speaks the local APIs of Shelly devices, without the
// Shelly cloud:
//
//   - GET /shelly (no login) tells what a device is and its generation
//     (Info).
//   - Gen1: HTTP GET with basic authentication (Client.Get1); the request
//     builders Gen1Turn, Gen1Level, Gen1Colour, Gen1Roller and
//     Gen1RollerPos; CoIoT, the CoAP status messages Gen1 devices
//     multicast (ParseCoAP, CitDesc, ListenCoIoT).
//   - Gen2, Gen3 and Gen4 (the RPC API): JSON-RPC over HTTP with SHA-256
//     digest authentication (Client.RPC), the WebSocket RPC channel with
//     its notifications (WatchWS, WSAuth), and a server for the outbound
//     WebSocket that sleeping devices open themselves (Inbound).
//   - MQTT mode for both, over the application's MQTT client (MQTT).
//   - First-time setup: the site's Wi-Fi from the device's hotspot
//     (Client.SetWiFi) or over BLE RPC (BLERPC, over the application's
//     Bluetooth stack through GATT); firmware updates by URL
//     (Client.Update, Client.WaitVersion).
//   - Typed views of Gen2+ status and configuration (Status, Config) and
//     typed calls over any transport (Caller): outputs, scripts,
//     schedules, webhooks, the key-value store and virtual components;
//     Gen1 settings and actions (Gen1Settings, Client.Gen1Actions).
//     Package discover finds devices by DNS-SD, CoIoT or a scan.
//
// The package logs nothing and keeps no state beyond a Client's digest
// challenge and an MQTT's pending requests. Networking goes through the
// *http.Client, Publisher and GATT the application gives it, except the
// UDP and TCP listeners it is asked to open. Simulated devices of each
// generation are in the module github.com/cB-Sigma/shelly-sim.
package shelly
