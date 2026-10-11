// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package shelly

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/cB-Sigma/shelly/bthome"
)

// Bluetooth sensors through Gen2+ devices. A Gen2+ device with Bluetooth
// reaches BTHome sensors (Shelly BLU and others) two ways:
//
//   - natively, through the BTHome component: BTHomeDevice components
//     (one per sensor, by address) and BTHomeSensor components (one per
//     reading), managed with the BTHome.* calls below and read as typed
//     views of the status;
//   - as a relay: a script on the device passes every advertisement it
//     hears to the application as a "ble.scan_result" event, which
//     ParseBLEScanResult reads and package bthome decodes (BLERelayScript
//     is such a script).

// ---- native BTHome components ----------------------------------------------------

// BTHomeDeviceConfig is a BTHomeDevice's configuration. Key (the sensor's
// AES key, 32 hexadecimal digits) can be set but is never read back.
type BTHomeDeviceConfig struct {
	ID   int            `json:"id,omitempty"`
	Name *string        `json:"name"`
	Addr string         `json:"addr"`
	Key  string         `json:"key,omitempty"`
	Meta map[string]any `json:"meta,omitempty"`
}

// BTHomeSensorConfig is a BTHomeSensor's configuration: which object id
// and which occurrence (Idx, from 0) of the device's packets it shows.
type BTHomeSensorConfig struct {
	ID    int            `json:"id,omitempty"`
	Name  *string        `json:"name"`
	Addr  string         `json:"addr"`
	ObjID int            `json:"obj_id"`
	Idx   int            `json:"idx"`
	Meta  map[string]any `json:"meta,omitempty"`
}

// BTHomeDeviceStatus is a BTHomeDevice's status.
type BTHomeDeviceStatus struct {
	ID          int      `json:"id"`
	RSSI        *float64 `json:"rssi"`
	Battery     *float64 `json:"battery"`
	PacketID    *float64 `json:"packet_id"`
	LastUpdated float64  `json:"last_updated_ts"`
	Key         *bool    `json:"key,omitempty"`
	Paired      *bool    `json:"paired,omitempty"`
	RPC         *bool    `json:"rpc,omitempty"`
	FWVer       string   `json:"fw_ver,omitempty"`
	Errors      []string `json:"errors,omitempty"`
}

// BTHomeSensorStatus is a BTHomeSensor's status: the latest value (a
// number, a string or a bool) and when it came.
type BTHomeSensorStatus struct {
	ID          int     `json:"id"`
	Value       any     `json:"value"`
	LastUpdated float64 `json:"last_updated_ts"`
}

// Float returns the value as a number (a bool as 0 or 1).
func (s BTHomeSensorStatus) Float() (float64, bool) {
	switch v := s.Value.(type) {
	case float64:
		return v, true
	case bool:
		if v {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// BTHomeDevice returns the status of bthomedevice:id.
func (s Status) BTHomeDevice(id int) (v BTHomeDeviceStatus, ok bool) {
	return v, s.Decode(idKey("bthomedevice", id), &v)
}

// BTHomeSensor returns the status of bthomesensor:id.
func (s Status) BTHomeSensor(id int) (v BTHomeSensorStatus, ok bool) {
	return v, s.Decode(idKey("bthomesensor", id), &v)
}

// BTHomeDevice returns the configuration of bthomedevice:id.
func (c Config) BTHomeDevice(id int) (v BTHomeDeviceConfig, ok bool) {
	return v, c.Decode(idKey("bthomedevice", id), &v)
}

// BTHomeSensor returns the configuration of bthomesensor:id.
func (c Config) BTHomeSensor(id int) (v BTHomeSensorConfig, ok bool) {
	return v, c.Decode(idKey("bthomesensor", id), &v)
}

// addedKey reads the key of a new component: devices answer "added" (the
// examples of the documentation) or "key" (its table).
func addedKey(raw json.RawMessage) (string, int, error) {
	var r struct {
		Added string `json:"added"`
		Key   string `json:"key"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", 0, err
	}
	k := r.Added
	if k == "" {
		k = r.Key
	}
	_, id, ok := SplitKey(k)
	if !ok {
		return k, 0, fmt.Errorf("no component key in %s", raw)
	}
	return k, id, nil
}

// BTHomeAddDevice adds a BTHomeDevice for the sensor at cfg.Addr
// (BTHome.AddDevice). id 0 lets the device choose (200 to 299). It
// returns the new component's key and id.
func BTHomeAddDevice(ctx context.Context, c Caller, cfg BTHomeDeviceConfig, id int) (string, int, error) {
	conf := map[string]any{"addr": cfg.Addr}
	if cfg.Name != nil {
		conf["name"] = *cfg.Name
	}
	if cfg.Key != "" {
		conf["key"] = cfg.Key
	}
	if cfg.Meta != nil {
		conf["meta"] = cfg.Meta
	}
	p := map[string]any{"config": conf}
	if id != 0 {
		p["id"] = id
	}
	raw, err := c.RPC(ctx, "BTHome.AddDevice", p)
	if err != nil {
		return "", 0, err
	}
	return addedKey(raw)
}

// BTHomeDeleteDevice removes bthomedevice:id (BTHome.DeleteDevice).
func BTHomeDeleteDevice(ctx context.Context, c Caller, id int) error {
	return call(ctx, c, "BTHome.DeleteDevice", map[string]any{"id": id}, nil)
}

// BTHomeAddSensor adds a BTHomeSensor for one object of a device already
// added (BTHome.AddSensor). id 0 lets the device choose.
func BTHomeAddSensor(ctx context.Context, c Caller, cfg BTHomeSensorConfig, id int) (string, int, error) {
	conf := map[string]any{"addr": cfg.Addr, "obj_id": cfg.ObjID, "idx": cfg.Idx}
	if cfg.Name != nil {
		conf["name"] = *cfg.Name
	}
	if cfg.Meta != nil {
		conf["meta"] = cfg.Meta
	}
	p := map[string]any{"config": conf}
	if id != 0 {
		p["id"] = id
	}
	raw, err := c.RPC(ctx, "BTHome.AddSensor", p)
	if err != nil {
		return "", 0, err
	}
	return addedKey(raw)
}

// BTHomeDeleteSensor removes bthomesensor:id (BTHome.DeleteSensor).
func BTHomeDeleteSensor(ctx context.Context, c Caller, id int) error {
	return call(ctx, c, "BTHome.DeleteSensor", map[string]any{"id": id}, nil)
}

// BTHomeStartDiscovery starts an active scan for BTHome devices
// (BTHome.StartDeviceDiscovery); seconds 0 keeps the device's default.
// Found devices come as "device_discovered" events of component "bthome"
// (ParseBTHomeDiscovered), and "discovery_done" ends the scan.
func BTHomeStartDiscovery(ctx context.Context, c Caller, seconds int) error {
	var p map[string]any
	if seconds > 0 {
		p = map[string]any{"duration": seconds}
	}
	return call(ctx, c, "BTHome.StartDeviceDiscovery", p, nil)
}

// BTHomeResetEncryptionCounter resets the device's counter for encrypted
// BTHome it sends (BTHome.ResetEncryptionCounter); change the keys too.
func BTHomeResetEncryptionCounter(ctx context.Context, c Caller) error {
	return call(ctx, c, "BTHome.ResetEncryptionCounter", nil, nil)
}

// BTHomeObjectInfo is the device's description of an object id.
type BTHomeObjectInfo struct {
	ObjID int    `json:"obj_id"`
	Name  string `json:"obj_name"`
	Type  string `json:"type"` // "sensor", "binary_sensor", ...
	Unit  string `json:"unit,omitempty"`
}

// BTHomeObjectInfos asks the device to describe object ids
// (BTHome.GetObjectInfos), every page.
func BTHomeObjectInfos(ctx context.Context, c Caller, ids []int) ([]BTHomeObjectInfo, error) {
	var out []BTHomeObjectInfo
	offset := 0
	for page := 0; page < 64; page++ {
		var r struct {
			Objects []BTHomeObjectInfo `json:"objects"`
			Infos   []struct {
				ObjID int    `json:"obj_id"`
				Name  string `json:"name"`
				Type  string `json:"type"`
				Unit  string `json:"unit"`
			} `json:"infos"`
			Offset int `json:"offset"`
			Count  int `json:"count"`
			Total  int `json:"total"`
		}
		p := map[string]any{"obj_ids": ids}
		if offset > 0 {
			p["offset"] = offset
		}
		if err := call(ctx, c, "BTHome.GetObjectInfos", p, &r); err != nil {
			return out, err
		}
		got := len(r.Objects) + len(r.Infos)
		out = append(out, r.Objects...)
		for _, o := range r.Infos {
			out = append(out, BTHomeObjectInfo{ObjID: o.ObjID, Name: o.Name, Type: o.Type, Unit: o.Unit})
		}
		if got == 0 || r.Total == 0 || r.Offset+got >= r.Total {
			break
		}
		offset = r.Offset + got
	}
	return out, nil
}

// BTHomeKnownObject is an object id a BTHomeDevice has seen, with the
// sensor component that shows it ("" when none does).
type BTHomeKnownObject struct {
	ObjID     int     `json:"obj_id"`
	Idx       int     `json:"idx"`
	Component *string `json:"component"`
}

// BTHomeKnownObjects lists the objects bthomedevice:id has seen
// (BTHomeDevice.GetKnownObjects).
func BTHomeKnownObjects(ctx context.Context, c Caller, id int) ([]BTHomeKnownObject, error) {
	var r struct {
		Objects []BTHomeKnownObject `json:"objects"`
	}
	err := call(ctx, c, "BTHomeDevice.GetKnownObjects", map[string]any{"id": id}, &r)
	return r.Objects, err
}

// BTHomeDiscovered is one "device_discovered" event of a discovery scan.
type BTHomeDiscovered struct {
	Addr      string     `json:"addr"`
	LocalName string     `json:"local_name"`
	RSSI      float64    `json:"rssi"`
	Encrypted bool       `json:"encrypted"`
	Shelly    *BLEMfData `json:"shelly_mfdata,omitempty"`
}

// ParseBTHomeDiscovered reads a "device_discovered" event (an Event from
// Events, component "bthome").
func ParseBTHomeDiscovered(e Event) (BTHomeDiscovered, bool) {
	if e.Name != "device_discovered" {
		return BTHomeDiscovered{}, false
	}
	var d BTHomeDiscovered
	raw, ok := e.Fields["device"]
	if !ok {
		return d, false
	}
	b, _ := json.Marshal(raw)
	return d, json.Unmarshal(b, &d) == nil && d.Addr != ""
}

// BTHomeDeviceEvent is a button or dimmer event of a BTHomeDevice
// ("single_push", "double_push", "long_push", "rotate_left", ...): which
// object occurrence sent it, the remote's channel (-1 when it has none)
// and the device's sensor statuses by object id.
type BTHomeDeviceEvent struct {
	Component string                          `json:"component"`
	ID        int                             `json:"id"`
	Event     string                          `json:"event"`
	Idx       int                             `json:"idx"`
	Channel   int                             `json:"channel"`
	Sensors   map[string][]BTHomeSensorStatus `json:"sensors"`
	TS        float64                         `json:"ts"`
}

// ParseBTHomeDeviceEvent reads an event of a bthomedevice component.
func ParseBTHomeDeviceEvent(e Event) (BTHomeDeviceEvent, bool) {
	if !strings.HasPrefix(e.Component, "bthomedevice:") {
		return BTHomeDeviceEvent{}, false
	}
	b, _ := json.Marshal(e.Fields)
	var d BTHomeDeviceEvent
	d.Channel = -1
	return d, json.Unmarshal(b, &d) == nil
}

// ---- relayed advertisements ---------------------------------------------------------

// BLEScanResultEvent is the event name a relay script emits for the
// advertisements it hears; BLERelayScript and the Home Assistant script
// use it.
const BLEScanResultEvent = "ble.scan_result"

// BLEAdvert is one advertisement a device relayed.
type BLEAdvert struct {
	Addr string // the sender's address, upper case with colons
	RSSI int
	Data []byte // the advertising data, then the scan response
}

// BTHome decodes the advertisement's BTHome service data (key is the
// sender's bind key, or nil).
func (a BLEAdvert) BTHome(key []byte) (*bthome.Packet, error) {
	return bthome.DecodeAdvertising(a.Data, a.Addr, key)
}

// Shelly reads the Shelly manufacturer data of the advertisement, if any.
func (a BLEAdvert) Shelly() (BLEMfData, bool) {
	ads, _ := bthome.ParseAD(a.Data)
	d, ok := bthome.Manufacturer(ads)[ShellyCompanyID]
	if !ok {
		return BLEMfData{}, false
	}
	return ParseBLEMfData(d)
}

// ParseBLEScanResult reads the data of a "ble.scan_result" event: version 1
// is one advertisement [1, addr, rssi, adv, scan response] and version 2
// a list [2, [[addr, rssi, adv, scan response], ...]], with the data in
// base64.
func ParseBLEScanResult(data json.RawMessage) ([]BLEAdvert, error) {
	var top []json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil || len(top) < 2 {
		return nil, errors.New("ble.scan_result: not a list with a version")
	}
	var version int
	if err := json.Unmarshal(top[0], &version); err != nil {
		return nil, fmt.Errorf("ble.scan_result: version: %w", err)
	}
	one := func(fields []json.RawMessage) (BLEAdvert, error) {
		if len(fields) < 4 {
			return BLEAdvert{}, errors.New("ble.scan_result: short entry")
		}
		var addr, adv, rsp string
		var rssi float64
		if json.Unmarshal(fields[0], &addr) != nil || json.Unmarshal(fields[1], &rssi) != nil ||
			json.Unmarshal(fields[2], &adv) != nil || json.Unmarshal(fields[3], &rsp) != nil {
			return BLEAdvert{}, errors.New("ble.scan_result: bad entry")
		}
		a, err := base64.StdEncoding.DecodeString(adv)
		if err != nil {
			return BLEAdvert{}, fmt.Errorf("ble.scan_result: advertisement: %w", err)
		}
		r, err := base64.StdEncoding.DecodeString(rsp)
		if err != nil {
			return BLEAdvert{}, fmt.Errorf("ble.scan_result: scan response: %w", err)
		}
		return BLEAdvert{Addr: strings.ToUpper(addr), RSSI: int(rssi), Data: append(a, r...)}, nil
	}
	switch version {
	case 1:
		a, err := one(top[1:])
		if err != nil {
			return nil, err
		}
		return []BLEAdvert{a}, nil
	case 2:
		var list [][]json.RawMessage
		if err := json.Unmarshal(top[1], &list); err != nil {
			return nil, fmt.Errorf("ble.scan_result: %w", err)
		}
		out := make([]BLEAdvert, 0, len(list))
		for _, f := range list {
			a, err := one(f)
			if err != nil {
				return out, err
			}
			out = append(out, a)
		}
		return out, nil
	}
	return nil, fmt.Errorf("ble.scan_result: version %d not known", version)
}

// ParseBLEScanEvent reads the advertisements of a "ble.scan_result"
// event (an Event from Events, raised by a script component); ok is false
// for other events.
func ParseBLEScanEvent(e Event) ([]BLEAdvert, bool, error) {
	if e.Name != BLEScanResultEvent {
		return nil, false, nil
	}
	b, err := json.Marshal(e.Fields["data"])
	if err != nil {
		return nil, true, err
	}
	a, err := ParseBLEScanResult(b)
	return a, true, err
}

// ScanResultData builds the data of a version 2 "ble.scan_result" event,
// for simulators and tests.
func ScanResultData(adverts []BLEAdvert) json.RawMessage {
	list := make([]any, 0, len(adverts))
	for _, a := range adverts {
		list = append(list, []any{strings.ToLower(a.Addr), a.RSSI, base64.StdEncoding.EncodeToString(a.Data), ""})
	}
	b, _ := json.Marshal([]any{2, list})
	return b
}

// BLERelayScript returns the code of a script that passes every
// advertisement the device hears to the application as a version 2
// "ble.scan_result" event (on the WebSocket channel, MQTT or the outbound
// WebSocket), batching up to 16 per event. active asks for scan
// responses too (active scanning uses more battery on the senders).
// Install it with CreateScript, PutScriptCode, SetScriptConfig (enable)
// and StartScript; Bluetooth must be on (BLE.SetConfig).
func BLERelayScript(active bool) string {
	return `// BLE relay (github.com/cB-Sigma/shelly): emits ` + BLEScanResultEvent + ` events
let q = [];
let t = null;
function flush() {
  t = null;
  while (q.length > 0) {
    Shelly.emitEvent("` + BLEScanResultEvent + `", [2, q.slice(0, 16)]);
    q = q.slice(16);
  }
}
function heard(ev, r) {
  if (ev !== BLE.Scanner.SCAN_RESULT || q.length > 64) return;
  q.push([r.addr, r.rssi, btoa(r.advData), btoa(r.scanRsp)]);
  if (t === null) t = Timer.set(100, false, flush);
}
BLE.Scanner.Start({duration_ms: -1, active: ` + strconv.FormatBool(active) + `});
BLE.Scanner.Subscribe(heard);
`
}

// ---- Shelly manufacturer data -------------------------------------------------------

// ShellyCompanyID is the Bluetooth company identifier in Shelly devices'
// manufacturer data.
const ShellyCompanyID = 0x0BA9

// Flags of the Shelly manufacturer data.
const (
	BLEFlagDiscoverable = 1 << 0
	BLEFlagAuth         = 1 << 1
	BLEFlagRPC          = 1 << 2 // RPC over Bluetooth is on
	BLEFlagBuzzer       = 1 << 3
	BLEFlagPairing      = 1 << 4
)

// BLEMfData is the Shelly manufacturer data of an advertisement: flags,
// the device's Wi-Fi MAC and a model id.
type BLEMfData struct {
	Flags   int    `json:"flags"`
	ModelID int    `json:"model_id"`
	MAC     string `json:"mac"`
}

// ParseBLEMfData reads Shelly manufacturer data (after the company id):
// blocks of a type byte and a fixed size (flags 2 bytes, MAC 6 bytes sent
// last byte first, model id 2 bytes). It stops at a block type it does not
// know.
func ParseBLEMfData(d []byte) (BLEMfData, bool) {
	var m BLEMfData
	found := false
	for i := 0; i < len(d); {
		typ := d[i]
		i++
		switch typ {
		case 0x01:
			if i+2 > len(d) {
				return m, found
			}
			m.Flags = int(binary.LittleEndian.Uint16(d[i:]))
			i += 2
		case 0x0A:
			if i+6 > len(d) {
				return m, found
			}
			parts := make([]string, 6)
			for k := 0; k < 6; k++ {
				parts[k] = fmt.Sprintf("%02X", d[i+5-k])
			}
			m.MAC = strings.Join(parts, ":")
			i += 6
		case 0x0B:
			if i+2 > len(d) {
				return m, found
			}
			m.ModelID = int(binary.LittleEndian.Uint16(d[i:]))
			i += 2
		default:
			return m, found
		}
		found = true
	}
	return m, found
}
