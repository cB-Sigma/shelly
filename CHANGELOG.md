# Changelog

All notable changes to this module. Versions follow semantic versioning;
before v1 the API may change in minor versions.

## v0.2.0 (2026-10-11)

Fills the v0.1.0 "left out" and "no" rows except the Shelly cloud. The
API only grows; code written for v0.1.0 builds unchanged.

### Added

- Typed views over Gen2+ status and configuration (`Status`, `Config`,
  `ParseStatus`, `ParseConfig`) for switch, cover, light, rgb, rgbw,
  cct, input (switch, analog and counter), em, emdata, em1, em1data,
  pm1, temperature, humidity, devicepower, sys and wifi; optional
  readings are pointers, and any component stays readable raw (`Decode`,
  `Keys`).
- `Caller`, with `CallerFunc`, `MQTTDevice` and `BLEDevice`, so every
  typed call runs over HTTP, MQTT, BLE or the application's own channel.
  `GetStatus`, `GetConfig`, `GetDeviceInfo`, `GetComponents` (every
  page; dynamic components only on request), `SetConfig`, `SetSwitch`,
  `ToggleSwitch`, `GetSwitch`, `GetCover`, `MoveCover`,
  `CoverToPosition`, `SetLight`, `IsRPCError`.
- Scripts (create, upload in chunks split between UTF-8 characters, read
  back page by page, start, stop, eval, status, config, delete),
  schedules, webhooks (and `WebhookEvents`, which reads both answer forms
  of Webhook.ListSupported), the key-value store (etags; both answer
  forms of KVS.GetMany) and virtual components.
- Gen1 settings beyond Wi-Fi: `Gen1Settings`, `Client.Settings1`, and the
  actions (`Client.Gen1Actions` reading URLs as strings or as objects
  with a time window, `Gen1SetAction`, `Gen1ActionNames`).
- Package `discover`: DNS-SD (`_shelly._tcp` and `_http._tcp`) through
  the application's mDNS browser, CoIoT listening and a subnet scan, each
  confirmed with GET /shelly. No new dependency.

### Changed

- `MethodPrefix` spells the namespaces that are not a capitalised type
  as devices name them: WiFi, KVS, BLE, MQTT, EM, EMData, EM1, EM1Data,
  PM1, DevicePower (it gave Wifi, Kvs, ... before). Switch, Light,
  Cover, RGB, RGBW and CCT are unchanged.
- An error answer to `MQTT.RPC` now wraps the `*RPCError`, so its code
  can be read with `errors.As` or `IsRPCError`; the message is the same.

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
