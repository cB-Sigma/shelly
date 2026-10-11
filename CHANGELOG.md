# Changelog

All notable changes to this module. Versions follow semantic versioning;
before v1 the API may change in minor versions.

## v0.1.0 (2026-10-11)

First release as a repository of its own, split from SigmaM2M.

- `Client`: GET /shelly (`Info`), Gen1 GETs with basic authentication
  (`Get1`, `Get`), Gen2+ JSON-RPC over HTTP with SHA-256 digest (`RPC`),
  `SetWiFi`, `Update`, `WaitVersion`, `OutboundWS`, `SetOutboundWS`,
  `Reboot`, `CitDescription`; `BaseURL`, `RPCError`, `ErrAuth`,
  `DigestResponse`, `WSAuth`.
- Gen1 request builders: `Gen1Turn`, `Gen1Level` (gain on /color/n for a
  light in colour mode), `Gen1Colour`, `Gen1Roller`, `Gen1RollerPos`;
  `MethodPrefix`, `SplitKey`.
- Push: `WatchWS` and `Events` (Gen2+ WebSocket channel), `Inbound` and
  `ServeInbound` (the outbound WebSocket of sleeping devices), CoIoT
  (`ParseCoAP`, `EncodeCoIoT`, `CitDesc.Decode`, `ListenCoIoT`), MQTT mode
  (`MQTT` over the application's `Publisher`, `ParseGen1Report`,
  `Gen1MQTT`).
- BLE RPC framing (`BLERPC` over the application's `GATT`), `WiFiConfig`.
- A /cit/d sensor whose `L` is a list of block IDs is read
  (`CitSensor.Blocks`; `L` holds the first); such a description failed to
  decode before the split.
- The simulator is the separate module github.com/cB-Sigma/shelly-sim;
  this module's tests and examples use a small in-test fake device.
- A test-only nested module (`interop/`) reads payloads recorded from real
  devices, published by aioshelly and Home Assistant.
