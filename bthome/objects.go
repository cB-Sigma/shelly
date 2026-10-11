// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package bthome

import "sort"

// Kind says how an object's bytes are read.
type Kind int

const (
	// Sensor is a number: an integer of Size bytes, little-endian, times
	// Factor.
	Sensor Kind = iota
	// Binary is an on or off state (one byte, 0 or 1).
	Binary
	// Event is a button, dimmer or command event (see Object.Event).
	Event
	// Text is a UTF-8 string with a length byte in front.
	Text
	// Raw is bytes with a length byte in front.
	Raw
	// Time is a Unix time in seconds, four bytes.
	Time
	// DeviceInfo is information about the sender (type, firmware), not a
	// reading.
	DeviceInfo
)

// String names the kind.
func (k Kind) String() string {
	switch k {
	case Sensor:
		return "sensor"
	case Binary:
		return "binary_sensor"
	case Event:
		return "event"
	case Text:
		return "text"
	case Raw:
		return "raw"
	case Time:
		return "timestamp"
	case DeviceInfo:
		return "device_info"
	}
	return "unknown"
}

// ObjectInfo describes one object id of the BTHome v2 format.
type ObjectInfo struct {
	ID     byte
	Name   string  // our own short name, such as "temperature"
	Unit   string  // "" for none
	Kind   Kind    //
	Size   int     // bytes of value; 0 for objects with a length byte
	Signed bool    // the integer is two's complement
	Factor float64 // the integer times Factor is the value (1 when not scaled)
}

func sensorObj(id byte, name, unit string, size int, signed bool, factor float64) ObjectInfo {
	return ObjectInfo{ID: id, Name: name, Unit: unit, Kind: Sensor, Size: size, Signed: signed, Factor: factor}
}

func binaryObj(id byte, name string) ObjectInfo {
	return ObjectInfo{ID: id, Name: name, Kind: Binary, Size: 1, Factor: 1}
}

// table is the object ids of the BTHome v2 format: the format's public
// description at bthome.io, checked against bthome-ble 3.24.0 (see
// PROVENANCE.md). The names are ours.
var table = func() map[byte]ObjectInfo {
	list := []ObjectInfo{
		sensorObj(0x00, "packet_id", "", 1, false, 1),
		sensorObj(0x01, "battery", "%", 1, false, 1),
		sensorObj(0x02, "temperature", "°C", 2, true, 0.01),
		sensorObj(0x03, "humidity", "%", 2, false, 0.01),
		sensorObj(0x04, "pressure", "hPa", 3, false, 0.01),
		sensorObj(0x05, "illuminance", "lx", 3, false, 0.01),
		sensorObj(0x06, "mass", "kg", 2, false, 0.01),
		sensorObj(0x07, "mass", "lb", 2, false, 0.01),
		sensorObj(0x08, "dewpoint", "°C", 2, true, 0.01),
		sensorObj(0x09, "count", "", 1, false, 1),
		sensorObj(0x0A, "energy", "kWh", 3, false, 0.001),
		sensorObj(0x0B, "power", "W", 3, false, 0.01),
		sensorObj(0x0C, "voltage", "V", 2, false, 0.001),
		sensorObj(0x0D, "pm2_5", "µg/m³", 2, false, 1),
		sensorObj(0x0E, "pm10", "µg/m³", 2, false, 1),
		binaryObj(0x0F, "generic_boolean"),
		binaryObj(0x10, "power_on"),
		binaryObj(0x11, "opening"),
		sensorObj(0x12, "co2", "ppm", 2, false, 1),
		sensorObj(0x13, "tvoc", "µg/m³", 2, false, 1),
		sensorObj(0x14, "moisture", "%", 2, false, 0.01),
		binaryObj(0x15, "battery_low"),
		binaryObj(0x16, "battery_charging"),
		binaryObj(0x17, "carbon_monoxide"),
		binaryObj(0x18, "cold"),
		binaryObj(0x19, "connectivity"),
		binaryObj(0x1A, "door"),
		binaryObj(0x1B, "garage_door"),
		binaryObj(0x1C, "gas"),
		binaryObj(0x1D, "heat"),
		binaryObj(0x1E, "light"),
		binaryObj(0x1F, "lock"),
		binaryObj(0x20, "moisture_detected"),
		binaryObj(0x21, "motion"),
		binaryObj(0x22, "moving"),
		binaryObj(0x23, "occupancy"),
		binaryObj(0x24, "plug"),
		binaryObj(0x25, "presence"),
		binaryObj(0x26, "problem"),
		binaryObj(0x27, "running"),
		binaryObj(0x28, "safety"),
		binaryObj(0x29, "smoke"),
		binaryObj(0x2A, "sound"),
		binaryObj(0x2B, "tamper"),
		binaryObj(0x2C, "vibration"),
		binaryObj(0x2D, "window"),
		sensorObj(0x2E, "humidity", "%", 1, false, 1),
		sensorObj(0x2F, "moisture", "%", 1, false, 1),
		{ID: 0x3A, Name: "button", Kind: Event, Size: 1, Factor: 1},
		{ID: 0x3B, Name: "command", Kind: Event, Size: 0, Factor: 1},
		{ID: 0x3C, Name: "dimmer", Kind: Event, Size: 2, Factor: 1},
		sensorObj(0x3D, "count", "", 2, false, 1),
		sensorObj(0x3E, "count", "", 4, false, 1),
		sensorObj(0x3F, "rotation", "°", 2, true, 0.1),
		sensorObj(0x40, "distance", "mm", 2, false, 1),
		sensorObj(0x41, "distance", "m", 2, false, 0.1),
		sensorObj(0x42, "duration", "s", 3, false, 0.001),
		sensorObj(0x43, "current", "A", 2, false, 0.001),
		sensorObj(0x44, "speed", "m/s", 2, false, 0.01),
		sensorObj(0x45, "temperature", "°C", 2, true, 0.1),
		sensorObj(0x46, "uv_index", "", 1, false, 0.1),
		sensorObj(0x47, "volume", "L", 2, false, 0.1),
		sensorObj(0x48, "volume", "mL", 2, false, 1),
		sensorObj(0x49, "volume_flow_rate", "m³/h", 2, false, 0.001),
		sensorObj(0x4A, "voltage", "V", 2, false, 0.1),
		sensorObj(0x4B, "gas", "m³", 3, false, 0.001),
		sensorObj(0x4C, "gas", "m³", 4, false, 0.001),
		sensorObj(0x4D, "energy", "kWh", 4, false, 0.001),
		sensorObj(0x4E, "volume", "L", 4, false, 0.001),
		sensorObj(0x4F, "water", "L", 4, false, 0.001),
		{ID: 0x50, Name: "timestamp", Kind: Time, Size: 4, Factor: 1},
		sensorObj(0x51, "acceleration", "m/s²", 2, false, 0.001),
		sensorObj(0x52, "gyroscope", "°/s", 2, false, 0.001),
		{ID: 0x53, Name: "text", Kind: Text, Factor: 1},
		{ID: 0x54, Name: "raw", Kind: Raw, Factor: 1},
		sensorObj(0x55, "volume_storage", "L", 4, false, 0.001),
		sensorObj(0x56, "conductivity", "µS/cm", 2, false, 1),
		sensorObj(0x57, "temperature", "°C", 1, true, 1),
		sensorObj(0x58, "temperature", "°C", 1, true, 0.35),
		sensorObj(0x59, "count", "", 1, true, 1),
		sensorObj(0x5A, "count", "", 2, true, 1),
		sensorObj(0x5B, "count", "", 4, true, 1),
		sensorObj(0x5C, "power", "W", 4, true, 0.01),
		sensorObj(0x5D, "current", "A", 2, true, 0.001),
		sensorObj(0x5E, "direction", "°", 2, false, 0.01),
		sensorObj(0x5F, "precipitation", "mm", 2, false, 0.1),
		sensorObj(0x60, "channel", "", 1, false, 1),
		sensorObj(0x61, "rotational_speed", "rpm", 2, false, 1),
		sensorObj(0x62, "speed", "m/s", 4, true, 0.000001),
		sensorObj(0x63, "acceleration", "m/s²", 4, true, 0.000001),
		sensorObj(0x64, "light_level", "", 1, false, 1),
		sensorObj(0x65, "settings_revision", "", 1, false, 1),
		{ID: 0xF0, Name: "device_type_id", Kind: DeviceInfo, Size: 2, Factor: 1},
		{ID: 0xF1, Name: "firmware_version", Kind: DeviceInfo, Size: 4, Factor: 1},
		{ID: 0xF2, Name: "firmware_version", Kind: DeviceInfo, Size: 3, Factor: 1},
	}
	m := make(map[byte]ObjectInfo, len(list))
	for _, o := range list {
		m[o.ID] = o
	}
	return m
}()

// Info describes object id, or reports false for an id this package does
// not know.
func Info(id byte) (ObjectInfo, bool) {
	o, ok := table[id]
	return o, ok
}

// Objects lists every known object id in order.
func Objects() []ObjectInfo {
	out := make([]ObjectInfo, 0, len(table))
	for _, o := range table {
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Event names, as the format numbers them.
var (
	buttonEvents  = map[byte]string{0x00: "", 0x01: "press", 0x02: "double_press", 0x03: "triple_press", 0x04: "long_press", 0x05: "long_double_press", 0x06: "long_triple_press", 0x80: "hold_press"}
	dimmerEvents  = map[byte]string{0x00: "", 0x01: "rotate_left", 0x02: "rotate_right"}
	commandEvents = map[byte]string{0x00: "off", 0x01: "on", 0x02: "toggle", 0x03: "step_up", 0x04: "step_down"}
)

// ButtonEvent is the code of a button event name ("press", "long_press",
// ...), for Builder.
func ButtonEvent(name string) (byte, bool) { return codeOf(buttonEvents, name) }

// DimmerEvent is the code of "rotate_left" or "rotate_right".
func DimmerEvent(name string) (byte, bool) { return codeOf(dimmerEvents, name) }

// CommandEvent is the code of a command ("on", "off", "toggle",
// "step_up", "step_down").
func CommandEvent(name string) (byte, bool) { return codeOf(commandEvents, name) }

func codeOf(m map[byte]string, name string) (byte, bool) {
	for c, n := range m {
		if n == name && n != "" {
			return c, true
		}
	}
	return 0, false
}
