# Provenance

## How the code was written

Written in Go for SigmaM2M, a oneM2M platform, and split out of its Shelly
integration as a module of its own. No code was ported or copied from
another implementation, and no source file of another project was
translated.

Shelly publishes its local APIs (shelly-api-docs.shelly.cloud). The code
follows those documents for facts: paths, method and field names, topic
layouts, option numbers, the authentication forms and the BLE
characteristic UUIDs. The Go types, the channel and request handling, the
error reporting and the simulator are our own design.

History:

- 2026-09-27: the HTTP client for Gen1 and Gen2+ (GET /shelly, basic and
  SHA-256 digest authentication, JSON-RPC over HTTP, the WebSocket
  authentication object), the WebSocket notification channel, BLE RPC
  framing and the simulator, as part of SigmaM2M's Shelly integration.
- 2026-10-05: CoIoT (`coiot.go`), the outbound-WebSocket server
  (`inbound.go`) and MQTT mode (`mqtt.go`). CoIoT sensor IDs differ
  between models, so values are always read through the device's /cit/d
  description, never by fixed IDs. The same day, the Gen1 level request
  for lights in colour mode (gain on /color/n), `Gen1Level`.
- 2026-10-06: the protocol code was moved out of the integration into this
  module: `WatchWS`, `ListenCoIoT`, `CitDesc.Decode`, the `MQTT` type with
  an injected `Publisher`, `Inbound` and `ServeInbound`, the request
  builders (`requests.go`), `SetWiFi`, `Update`, `WaitVersion`,
  `OutboundWS` and `SetOutboundWS`. The integration keeps the mapping onto
  its own model, its settings, warnings and discovery.
- 2026-10-11: the simulator moved to its own module,
  github.com/cB-Sigma/shelly-sim, with the tests that drive it. This
  module keeps its unit tests, the RFC 7616 vector and runnable examples
  against a small in-test fake device (`fake_test.go`). A /cit/d sensor
  whose `L` is a list of blocks is now read (`CitSensor.Blocks`); it made
  the whole description fail to decode before. Interop tests against
  recorded real-device payloads were added (below).

- 2026-10-11 (v0.2.0): typed views (`components.go`), the `Caller` calls
  with scripts, schedules, webhooks, KVS and virtual components
  (`calls.go`), Gen1 settings and actions (`gen1settings.go`) and the
  `discover` package. Field names and method names follow Shelly's
  component documents; the two answer forms of Webhook.ListSupported and
  KVS.GetMany, and the two URL forms of Gen1 actions, follow what those
  documents give for older and newer firmware. That Script.GetCode reads
  in pages by byte offset, and that aioshelly decides script support from
  Shelly.ListMethods, was read from aioshelly 13.34.1's RPC device code
  (facts only, no code taken).

Files marked "Copyright 2026 Abhinav Gautam" were written or rewritten
for this module.

## Sources

| Area (files) | Followed (facts) |
|---|---|
| GET /shelly, generations (client.go) | Shelly API docs: Gen1 /shelly, Gen2+ Shelly.GetDeviceInfo and /shelly |
| Gen1 HTTP, requests (client.go, requests.go) | Gen1 API docs: /status, /settings, /relay, /light, /color, /roller, /settings/sta, /ota, basic authentication |
| Gen2+ RPC and digest (client.go) | Gen2+ docs: RPC protocol, authentication (SHA-256 digest, user admin); RFC 7616 |
| WebSocket channel and auth object (ws.go, client.go `WSAuth`) | Gen2+ docs: RPC channels (WebSocket), notifications, the authentication object over WebSocket |
| Outbound WebSocket (inbound.go) | Gen2+ docs: the Ws component (Ws.GetConfig, Ws.SetConfig, ssl_ca) |
| CoIoT (coiot.go) | Gen1 docs: CoIoT v2 (options 3332, 3412, 3420; /cit/s, /cit/d); RFC 7252 |
| MQTT (mqtt.go) | Gen1 docs: MQTT topics and commands; Gen2+ docs: MQTT channel (events/rpc, rpc, online) |
| BLE RPC (ble.go) | Gen2+ docs: BLE RPC service and characteristics |
| Firmware (firmware.go) | Gen1 /ota; Gen2+ Shelly.Update |
| Typed views (components.go) | Gen2+ docs: the status and configuration of Switch, Cover, Light, RGB, RGBW, CCT, Input, EM, EMData, EM1, EM1Data, PM1, Temperature, Humidity, DevicePower, Sys, WiFi |
| Calls (calls.go) | Gen2+ docs: Shelly.GetComponents, Script, Schedule, Webhook, KVS, Virtual and the virtual component types |
| Gen1 settings (gen1settings.go) | Gen1 docs: /settings, /settings/relay, /settings/roller, /settings/light, /settings/actions |
| Discovery (discover/) | Gen2+ docs: mDNS service and TXT keys; Gen1 docs: CoIoT; RFC 6763 (DNS-SD) |

The list form of a /cit/d sensor's `L` was learnt from reading aioshelly's
block device code (facts only).

## Independent verification

Run on 2026-10-11:

- `interop/` (this module's client, recorded payloads): the four Gen1 and
  nine Gen2 device dumps of aioshelly 13.34.1's `fixtures/`, its recorded
  Plus 2PM device info with authentication on and WebSocket 401 frame,
  and the twelve Gen2, Gen3 and Gen4 device dumps of Home Assistant core's
  Shelly test fixtures at commit e8c451ce3967. A test server answers
  with the recorded payloads; the client's `Shelly`, `Get1`, `RPC`,
  `OutboundWS`, `SplitKey`, `MethodPrefix` and `WSAuth` read them. All
  passed; no client bug was found this way. The fixtures are anonymised
  dumps contributed to those projects, not captures made by us, and the
  serving is ours.
- github.com/cB-Sigma/shelly-sim's `interop/` ran aioshelly against the
  simulator; the bugs it found were in the simulator, and are recorded
  there.

For v0.2.0, aioshelly 13.34.1 also ran its script, KVS, virtual
component and dynamic component calls against shelly-sim (35 checks,
all passed, after a simulator deadlock on Number.Set over WebSocket was
fixed), and this module's typed calls run the same flows in shelly-sim's
tests; this module's MQTT type ran against simulated devices on an
embedded mochi-mqtt broker, and `discover` against DNS-SD records
announced over multicast by github.com/cB-Sigma/mdns.

Not done: a real device, and an independent device emulator. The open
Shelly emulators found on GitHub on 2026-10-11 each imitate one energy
meter's readings for a particular battery or inverter, not the device
APIs this module speaks, and were not used.

## Passages to review before publication

No passage is known to be copied. The documentation is the only source,
so a check is about prose: the package and type comments describe the
protocol in our own words and cite the documents by name.
