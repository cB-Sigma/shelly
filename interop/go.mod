// Test-only module: this module's client against recorded payloads of
// real Shelly devices published by aioshelly and Home Assistant (see
// doc.go). Not part of the published module.
module github.com/cB-Sigma/shelly/interop

go 1.24

replace github.com/cB-Sigma/shelly => ../

require github.com/cB-Sigma/shelly v0.0.0

require github.com/coder/websocket v1.8.15 // indirect
