# shelly

The local APIs of Shelly devices in pure Go, without the Shelly cloud; not affiliated with Shelly Group.

[![Go Reference](https://pkg.go.dev/badge/github.com/cB-Sigma/shelly.svg)](https://pkg.go.dev/github.com/cB-Sigma/shelly) [![test](https://github.com/cB-Sigma/shelly/actions/workflows/test.yml/badge.svg)](https://github.com/cB-Sigma/shelly/actions/workflows/test.yml) [![Licence: Apache-2.0](https://img.shields.io/badge/licence-Apache--2.0-blue.svg)](LICENSE) [![Go](https://img.shields.io/badge/go-1.24-00ADD8.svg)](go.mod)

It speaks Gen1 HTTP with basic authentication, CoIoT and MQTT, and the
Gen2, Gen3 and Gen4 JSON-RPC API over HTTP with SHA-256 digest
authentication, over the WebSocket notification channel, over the
outbound WebSocket that sleeping devices open, over MQTT and over BLE; it
also sets up a new device's Wi-Fi and updates firmware by URL, receives
webhooks and action URLs, and reads Shelly BLU and other BTHome
Bluetooth sensors (package `bthome`, with AES-CCM). Its one
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
| Typed status and configuration: switch, cover, light/rgb/rgbw/cct, input (switch, analog, counter), em, emdata, em1, em1data, pm1, temperature, humidity, devicepower, sys, wifi | yes, as views over the maps (`Status`, `Config`); unknown components and fields stay readable |
| Typed calls over any transport (`Caller`: HTTP, MQTT, BLE, your own) | yes: status, config, `<Type>.SetConfig`, switch, cover, light, components with paging |
| Scripts: create, upload in chunks, read back in pages, start, stop, eval, status, config, delete | yes |
| Schedules, webhooks (with the supported-event list), KVS (with etags, both GetMany forms), virtual components | yes |
| Gen1 settings beyond Wi-Fi: /settings and /settings/<section>/<n>, actions (/settings/actions, both URL forms) | yes |
| Discovery: DNS-SD (`_shelly._tcp`, `_http._tcp`) through your mDNS browser, CoIoT, a subnet scan; each confirmed with GET /shelly | yes, package `discover` |
| Webhook and action URL receiver: an `http.Handler` for Gen2+ webhooks (with `${...}` URL tokens), Gen1 actions and Gen1 sensor report URLs, with a shared token and a source check (`HookHandler`, `HookURL`, `HookWebhook`) | yes, since v0.3.0 |
| BTHome (Shelly BLU and other BTHome sensors): v2 and v1 decoding, AES-CCM encryption, every object id of the format, events, a packet builder, advertising data, replay guard | yes, package `bthome`, since v0.3.0 |
| Bluetooth through Gen2+ devices: the BTHome component (add and delete devices and sensors, discovery, object information, known objects, events, typed status), relayed advertisements (`ble.scan_result`, with a relay script), Shelly manufacturer data | yes, since v0.3.0 |
| Typed views of smoke, flood, illuminance, voltmeter | yes, since v0.3.0 |
| The Shelly cloud | no: needs a Shelly cloud account and its auth key (see Not yet) |

Left out on purpose:

- An mDNS implementation, a Bluetooth stack and an MQTT client: they come
  from the application, which keeps this module free of cgo and heavy
  dependencies. `discover.Browser` takes any mDNS browser;
  [mdns](https://github.com/cB-Sigma/mdns) is one (below).

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
| Typed component models | Gen2+ status and configuration of the common components, as views over the maps; Gen1 as maps | yes, Gen1 and Gen2+ | yes, Gen2+ |
| Scripts, schedules, webhooks, KVS, virtual components | yes | yes | types only |
| Gen1 settings and actions | yes | yes | no |
| Discovery | DNS-SD over your mDNS browser, CoIoT, subnet scan | mDNS, CoIoT, BLE, Wi-Fi | no |
| Webhook and action URL receiver | yes (`HookHandler`) | no (creates webhooks only) | no |
| BTHome decoding | every object id of the format, v1 and v2, AES-CCM decryption, builder; checked against bthome-ble | nine object ids, no decryption | no |
| BTHome component of Gen2+ devices | calls, views and events | calls and types | no |
| Relayed advertisements (`ble.scan_result`) | yes, with a relay script | no | no |
| Cloud API | no | yes | no |
| Simulator for tests | yes ([shelly-sim](https://github.com/cB-Sigma/shelly-sim)) | mocks and one fixture | no |
| cgo | none | on macOS, through tinygo bluetooth | none |
| Direct dependencies | coder/websocket | 6 (Paho, gorilla/websocket, tinygo bluetooth, gonetworkmanager, mdlayher/wifi, testcontainers-go) | 4 (Paho, google/uuid, mongoose-os/mos, testify) |
| Size (non-test Go lines) | about 5,400 | about 61,600 | about 6,200 |

Pick this module for a gateway or integration that has to reach every
generation, take pushes by every route (CoIoT, the WebSocket channel,
sleeping devices' outbound WebSocket, MQTT) and stay small and free of
cgo, with typed views where you want them and the raw maps where a new
component appears, receive webhooks, or read Shelly BLU and other
BTHome sensors (through a Shelly device or your own Bluetooth scanner).
Pick tj-smith47/shelly-go for the Shelly cloud, BLE scanning and Wi-Fi
discovery on its own stacks, or typed models of every Gen1 endpoint and
rarer Gen2+ components. Pick jcodybaker/go-shelly if you build on mongoose-os's RPC
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

Typed views and the newer calls take any `Caller` (a `*Client`, an
`MQTTDevice`, a `BLEDevice`):

```go
st, _ := shelly.GetStatus(ctx, c)
if sw, ok := st.Switch(0); ok && sw.APower != nil {
	fmt.Println(sw.Output, *sw.APower)
}
id, _ := shelly.CreateScript(ctx, c, "blink")
_ = shelly.PutScriptCode(ctx, c, id, code, 0) // in chunks
_, _ = shelly.StartScript(ctx, c, id)
_, _, _ = shelly.KVSSet(ctx, c, "mode", "eco", "")
```

Discovery with [mdns](https://github.com/cB-Sigma/mdns) as the browser:

```go
browse := func(ctx context.Context, svc string) ([]discover.Service, error) {
	list, err := mdns.Browse(ctx, svc, mdns.Options{})
	out := make([]discover.Service, 0, len(list))
	for _, s := range list {
		out = append(out, discover.Service{Instance: s.Instance, Host: s.Host, Port: s.Port, IPs: s.IPs, TXT: s.TXT})
	}
	return out, err
}
devices, err := discover.MDNS(ctx, browse, discover.Options{})
```

Receiving webhooks, and BTHome sensors heard by a Gen2+ device:

```go
http.Handle("/hooks/", &shelly.HookHandler{Prefix: "/hooks", Token: secret,
	Handle: func(h shelly.Hook) { tC, _ := h.Float("tC"); log.Println(h.Device, h.Event, tC) }})
_, _, _ = shelly.CreateWebhook(ctx, c, shelly.HookWebhook("http://192.168.1.10:8080/hooks", id,
	"temperature.change", 0, map[string]string{"tC": "${ev.tC}"}, secret))

_, devID, _ := shelly.BTHomeAddDevice(ctx, c, shelly.BTHomeDeviceConfig{Addr: "3c:2e:f5:71:d5:2a", Key: bindKey}, 0)
st, _ := shelly.GetStatus(ctx, c)
d, _ := st.BTHomeDevice(devID) // battery, RSSI, packet id

// Or decode advertisements yourself (relayed by BLERelayScript, or from your scanner):
p, err := bthome.DecodeAdvertising(adv, "3C:2E:F5:71:D5:2A", key)
```

The runnable examples are `Example`, `ExampleGen1Level` and
`ExampleParseCoAP` in example_test.go, and `ExampleDecode` in
bthome/; `go test` runs them.

## Documentation

API reference: https://pkg.go.dev/github.com/cB-Sigma/shelly

| Package | What it does |
|---|---|
| `shelly` (module root) | `Client` (`Shelly`, `Get1`, `Get`, `RPC`, `SetWiFi`, `Update`, `WaitVersion`, `OutboundWS`, `SetOutboundWS`, `Reboot`, `CitDescription`), `Info`, `RPCError`, `ErrAuth`, `DigestResponse`, `WSAuth`, `BaseURL`. Push: `WatchWS` and `Events` (Gen2+ WebSocket), `Inbound` and `ServeInbound` (outbound WebSocket), `ParseCoAP`, `EncodeCoIoT`, `CitDesc.Decode`, `ListenCoIoT` (CoIoT), `MQTT` with `Publisher`, `ParseGen1Report`, `Gen1MQTT` (MQTT mode). Requests: `Gen1Turn`, `Gen1Level`, `Gen1Colour`, `Gen1Roller`, `Gen1RollerPos`, `MethodPrefix`, `SplitKey`. Setup: `BLERPC` over a `GATT`, `WiFiConfig` |
| `shelly`, typed views and calls | `Status` and `Config` (`ParseStatus`, `ParseConfig`, `Switch`, `Cover`, `Light`, `Input`, `EM`, `EMData`, `EM1`, `EM1Data`, `PM1`, `Temperature`, `Humidity`, `DevicePower`, `Sys`, `WiFi`, `Keys`, `Decode`); `Caller`, `CallerFunc`, `MQTTDevice`, `BLEDevice`, `IsRPCError`; `GetStatus`, `GetConfig`, `GetDeviceInfo`, `GetComponents`, `SetConfig`, `SetSwitch`, `ToggleSwitch`, `GetSwitch`, `GetCover`, `MoveCover`, `CoverToPosition`, `SetLight`; scripts (`ListScripts`, `CreateScript`, `PutScriptCode`, `GetScriptCode`, `StartScript`, `StopScript`, `EvalScript`, `GetScriptStatus`, `SetScriptConfig`, `DeleteScript`), schedules (`CreateSchedule`, `UpdateSchedule`, `ListSchedules`, `DeleteSchedule`, `DeleteAllSchedules`), webhooks (`CreateWebhook`, `UpdateWebhook`, `ListWebhooks`, `DeleteWebhook`, `DeleteAllWebhooks`, `WebhookEvents`), KVS (`KVSSet`, `KVSGet`, `KVSGetMany`, `KVSList`, `KVSDelete`), virtual components (`AddVirtual`, `DeleteVirtual`, `SetVirtual`); Gen1 `Gen1Settings`, `Client.Settings1`, `Client.Gen1Actions`, `Gen1SetAction`, `Gen1ActionNames` |
| `shelly`, receiving and Bluetooth (v0.3.0) | `HookHandler`, `Hook`, `HookURL`, `HookWebhook`, the Gen1 action names; `BTHomeAddDevice`, `BTHomeAddSensor`, `BTHomeDeleteDevice`, `BTHomeDeleteSensor`, `BTHomeStartDiscovery`, `BTHomeObjectInfos`, `BTHomeKnownObjects`, `BTHomeResetEncryptionCounter`, `Status.BTHomeDevice`/`BTHomeSensor`, `Config.BTHomeDevice`/`BTHomeSensor`, `ParseBTHomeDiscovered`, `ParseBTHomeDeviceEvent`, `ParseBLEScanResult`, `ParseBLEScanEvent`, `ScanResultData`, `BLEAdvert`, `BLERelayScript`, `ParseBLEMfData`; `Status.Smoke`, `Flood`, `Illuminance`, `Voltmeter` |
| `bthome` | `Decode`, `DecodeAdvertising`, `Packet`, `Object`, `Info`, `Objects`, `Builder` (`Add`, `Button`, `Dimmer`, `Command`, `Text`, `Raw`, `Firmware`, `ServiceData`, `Seal`), `ParseAD`, `ServiceData16`, `Manufacturer`, `LocalName`, `Advertising`, `Guard`, `ParseMAC` |
| `discover` | `MDNS` over a `Browser`, `Scan`, `ListenCoIoT`, `Probe`, `FromService`, `FromCoIoT` |
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
not depend on its simulator. The typed views are tested against status
and configuration documents in the shape devices send, and the newer
calls against a recording `Caller`; `discover` against test servers on
loopback and a CoIoT socket. Two tests carry `// Covers: SIG-...`
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
  [shelly-sim](https://github.com/cB-Sigma/shelly-sim), including, for
  v0.2.0, scripts, KVS, virtual components, schedules, webhooks,
  per-component configuration, Gen3 and Gen4 devices, a Pro 3EM and a
  timed cover: checks the simulator this module's integration tests use.
  The same simulator behaviours are then driven by this module's typed
  calls in shelly-sim's tests, so the two clients agree on them.
- bthome-ble 3.24.0 (MIT; the BTHome parser Home Assistant uses), in
  python:3.12.10-slim in Docker (`interop`, 2026-10-12): package `bthome`
  built a packet for every object id it knows (91), for every button,
  dimmer and command event (14) and one encrypted packet with several
  objects and one with the address in the payload; bthome-ble decoded all
  106 and every value, event and argument agreed with package `bthome`'s
  own decoding; bthome-ble verified both bind keys. The worked examples
  of the format's description and RFC 3610's CCM vector are unit tests.
- aioshelly 13.34.1 with bthome-ble 3.24.0 against a simulated Gen3
  device (shelly-sim's `interop`, 2026-10-12): aioshelly installed its own
  Bluetooth relay script, the device relayed a simulated BLU door
  sensor's advertisement as `ble.scan_result` events, aioshelly parsed
  them and read the Shelly manufacturer data, bthome-ble decoded the
  BTHome data, and aioshelly added a BTHomeDevice and BTHomeSensor through
  `call_rpc` and saw the sensor's value in its status. This module's
  calls drive the same simulated BTHome component in shelly-sim's tests.
- The webhook receiver ran against shelly-sim's devices calling their
  Gen2+ webhooks (URL tokens replaced by the simulator) and Gen1 action
  and report URLs; no independent device calls webhooks, so that is our
  own reading of the documents on both ends.
- shelly-sim's tests also run this module's `MQTT` type against simulated
  devices connected to a real broker (mochi-mqtt v2.7.9, embedded), and
  `discover.MDNS` against the devices' DNS-SD records announced over real
  multicast by [mdns](https://github.com/cB-Sigma/mdns) (shelly-sim's
  `interop`, 2026-10-11).
- Not yet, needing hardware: a real device on a network (each
  generation), BLE RPC against a device, CoIoT multicast from a device,
  MQTT with real devices on a broker, a script actually run by a
  device's script engine (including `BLERelayScript`), a real Shelly BLU
  sensor's advertisements, webhooks called by a real device.
- Not yet, needing a vendor account: the Shelly cloud API (its
  server-side endpoints need an account's auth key and server name, so
  there is nothing to test against without one).

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
