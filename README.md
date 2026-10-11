# shelly

The local APIs of Shelly devices in pure Go, without the Shelly cloud; not affiliated with Shelly Group.

[![Go Reference](https://pkg.go.dev/badge/github.com/cB-Sigma/shelly.svg)](https://pkg.go.dev/github.com/cB-Sigma/shelly) [![test](https://github.com/cB-Sigma/shelly/actions/workflows/test.yml/badge.svg)](https://github.com/cB-Sigma/shelly/actions/workflows/test.yml) [![Licence: Apache-2.0](https://img.shields.io/badge/licence-Apache--2.0-blue.svg)](LICENSE) [![Go](https://img.shields.io/badge/go-1.24-00ADD8.svg)](go.mod)

It speaks Gen1 HTTP with basic authentication, CoIoT and MQTT, and the
Gen2, Gen3 and Gen4 JSON-RPC API over HTTP with SHA-256 digest
authentication, over the WebSocket notification channel, over the
outbound WebSocket that sleeping devices open, over MQTT and over BLE; it
also sets up a new device's Wi-Fi and updates firmware by URL. Its one
dependency is coder/websocket; there is no cgo. It was written for
SigmaM2M, a oneM2M platform, whose Shelly integration it serves.

## Features

| Feature | Status |
|---|---|
| GET /shelly, generation detection (`Info`) | yes |
| Gen1 HTTP with basic authentication; relay, light (white and colour mode), colour and roller requests | yes; a level on a light in colour mode is sent as gain on /color/n |
| Gen2+ JSON-RPC over HTTP with SHA-256 digest (RFC 7616; nonce count, one re-challenge) | yes |
| Gen2+ WebSocket channel: authentication object, status, notifications, keep-alive (`WatchWS`) | yes |
| Outbound WebSocket of sleeping Gen2+ devices: server (`Inbound`), Ws.GetConfig and Ws.SetConfig | yes |
| CoIoT: CoAP codec, /cit/d description (sensors in one block or several), multicast and unicast listener | yes; values by the description, never by fixed sensor IDs |
| MQTT mode: Gen1 topics and commands, Gen2+ notifications and JSON-RPC | yes, over the application's MQTT client (`Publisher`, `Filters`, `Handle`) |
| BLE RPC framing | yes, over the application's Bluetooth stack (`GATT`) |
| Wi-Fi setup (Gen1 /settings/sta, Gen2+ WiFi.SetConfig), firmware update by URL | yes |

Left out on purpose:

- Discovery. Gen2+ devices announce `_shelly._tcp` over mDNS; browse with
  an mDNS library such as [mdns](https://github.com/cB-Sigma/mdns). Gen1
  devices are also found by their CoIoT messages (`ListenCoIoT`).
- Typed per-component models. Status and configuration stay maps by
  component key (`switch:0`), so new components and fields reach the
  application without a new release.
- A Bluetooth stack and an MQTT client: they come from the application,
  which keeps this module free of cgo and heavy dependencies.
- The Shelly cloud, scripts, schedules, webhooks, Gen1 settings beyond
  Wi-Fi.

## Choosing a Go Shelly library

Compared from each library's source at the versions named, read on
2026-10-11 (not run). Check the projects themselves before relying on a
row.

| | this module | tj-smith47/shelly-go v0.14.0 | jcodybaker/go-shelly (commit 6c8a73a, 2026-07-19) |
|---|---|---|---|
| Licence | Apache-2.0 | Apache-2.0 | MIT |
| Gen1 HTTP with basic authentication | yes | yes | no (Gen2+ only) |
| Gen1 CoIoT | yes | yes | no |
| Gen1 MQTT topic model | yes | no (MQTT transport, topics by the caller) | no |
| Gen2+ RPC over HTTP with SHA-256 digest | yes, own code | yes, own code | through mongoose-os's mgrpc |
| Gen2+ WebSocket channel with notifications and the auth object | yes | yes | types only; transport from mgrpc |
| Server for the outbound WebSocket of sleeping devices | yes | no (Ws.SetConfig only) | no |
| Gen2+ RPC over MQTT | yes, over your client | yes, on Eclipse Paho | partial (no events/rpc) |
| BLE RPC | framing over your GATT | yes, on tinygo bluetooth | no |
| Typed component models | no, maps | yes, Gen1 and Gen2+ | yes, Gen2+ |
| Discovery, cloud API | no | mDNS, CoIoT, BLE, Wi-Fi; cloud | no |
| Simulator for tests | yes ([shelly-sim](https://github.com/cB-Sigma/shelly-sim)) | mocks and one fixture | no |
| cgo | none | on macOS, through tinygo bluetooth | none |
| Direct dependencies | coder/websocket | 6 (Paho, gorilla/websocket, tinygo bluetooth, gonetworkmanager, mdlayher/wifi, testcontainers-go) | 4 (Paho, google/uuid, mongoose-os/mos, testify) |
| Size (non-test Go lines) | about 1,800 | about 61,600 | about 6,200 |

Pick this module for a gateway or integration that has to reach every
generation, take pushes by every route (CoIoT, the WebSocket channel,
sleeping devices' outbound WebSocket, MQTT) and stay small and free of
cgo, and that is happy with status as maps. Pick tj-smith47/shelly-go for
typed models of each component, discovery and the Shelly cloud in one
package. Pick jcodybaker/go-shelly if you build on mongoose-os's RPC
tooling and need only Gen2+ devices.

## Install

```
go get github.com/cB-Sigma/shelly@latest
```

Requires Go 1.24 or later.

## Quick start

```go
import "github.com/cB-Sigma/shelly"

base, _ := shelly.BaseURL("192.168.1.50")
c := &shelly.Client{Base: base, Password: pw, HTTP: &http.Client{Timeout: 5 * time.Second}}
info, err := c.Shelly(ctx)
if info.Generation() >= 2 {
	_, err = c.RPC(ctx, "Switch.Set", map[string]any{"id": 0, "on": true})
	err = shelly.WatchWS(ctx, base, shelly.WSOptions{Src: "my-app", Password: pw,
		Notify: func(method string, params json.RawMessage) error { /* ... */ return nil }})
} else {
	_, err = c.Get1(ctx, shelly.Gen1Level(0, 40, colourMode, nil))
}
```

The runnable examples are `Example`, `ExampleGen1Level` and
`ExampleParseCoAP` in example_test.go; `go test` runs them.

## Documentation

API reference: https://pkg.go.dev/github.com/cB-Sigma/shelly

| Package | What it does |
|---|---|
| `shelly` (module root) | `Client` (`Shelly`, `Get1`, `Get`, `RPC`, `SetWiFi`, `Update`, `WaitVersion`, `OutboundWS`, `SetOutboundWS`, `Reboot`, `CitDescription`), `Info`, `RPCError`, `ErrAuth`, `DigestResponse`, `WSAuth`, `BaseURL`. Push: `WatchWS` and `Events` (Gen2+ WebSocket), `Inbound` and `ServeInbound` (outbound WebSocket), `ParseCoAP`, `EncodeCoIoT`, `CitDesc.Decode`, `ListenCoIoT` (CoIoT), `MQTT` with `Publisher`, `ParseGen1Report`, `Gen1MQTT` (MQTT mode). Requests: `Gen1Turn`, `Gen1Level`, `Gen1Colour`, `Gen1Roller`, `Gen1RollerPos`, `MethodPrefix`, `SplitKey`. Setup: `BLERPC` over a `GATT`, `WiFiConfig` |
| `interop` (nested module, tests only) | the client against payloads recorded from real devices (see below) |

Changes per version are in [CHANGELOG.md](CHANGELOG.md).

## Testing

```
go test -race ./...
```

The tests here use a small fake device written in the test
(`fake_test.go`) and the RFC 7616 SHA-256 example. The tests that drive
the full simulator (every request, CoIoT, MQTT, the WebSocket channel,
the outbound WebSocket, BLE, Wi-Fi setup and firmware updates) are in
[shelly-sim](https://github.com/cB-Sigma/shelly-sim), so this module does
not depend on its simulator. Two tests carry `// Covers: SIG-...`
comments: these are requirement IDs of the SigmaM2M project's compliance
report and have no meaning outside it.

`interop/` is a nested module with its own go.mod. Its tests fetch the
recorded payloads from GitHub at fixed revisions and are skipped unless
`SIGMA_SHELLY_INTEROP=1` is set:

```
cd interop && SIGMA_SHELLY_INTEROP=1 go test -v ./...
```

### Interop tested against

- Payloads recorded from real devices (`interop`, 2026-10-11): the four
  Gen1 devices (Shelly Air, Gas, Plug S, Vintage) and nine Gen2 devices
  (BLU Gateway at two firmware versions, Plus 1PM, Plus Plug IT, Plus
  Plug S, Pro 3EM, Pro 3, Pro 4PM, Wall Display) of aioshelly 13.34.1's
  fixtures, its Plus 2PM device info with authentication on and the
  WebSocket 401 frame that device sent, and the twelve Gen2, Gen3 and
  Gen4 devices of Home Assistant core's Shelly test fixtures (commit
  e8c451ce3967; among them a 2PM Gen3 as switches and as a cover, a Pro
  3EM, a Duo Bulb Gen3, a Gen4 power strip, a Wall Display XL). A test
  server answers with the recorded payloads; `Shelly`, `Get1`, `RPC`,
  `OutboundWS`, `SplitKey`, `MethodPrefix` and `WSAuth` read them
  correctly. The dumps are anonymised recordings made by those projects'
  contributors; the serving is ours.
- aioshelly 13.34.1 against the simulator in
  [shelly-sim](https://github.com/cB-Sigma/shelly-sim): checks the
  simulator this module's integration tests use, not this module.
- Not yet: a real device on a network, BLE RPC against a device, MQTT
  against a broker with devices, CoIoT multicast from a device.

## Related repositories

- [shelly-sim](https://github.com/cB-Sigma/shelly-sim): simulated Gen1
  and Gen2+ devices on their local APIs, as a Go package and a command,
  with this module's integration tests.

## Status

v0.x: the API may change before v1.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

See [SECURITY.md](SECURITY.md) to report a vulnerability privately.

## Licence

Apache-2.0 (see [LICENSE](LICENSE) and [NOTICE](NOTICE)). Copyright 2026
Abhinav Gautam. Shelly is a trademark of Shelly Group; the name is used
only to say which devices this module talks to.
