// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Package bthome reads and writes BTHome advertisements: the open
// Bluetooth LE sensor format that Shelly BLU devices (buttons, door and
// window, motion, H&T, remote, wall switch, distance) and many DIY
// sensors send. It decodes version 2 (service UUID 0xFCD2, with or
// without AES-CCM encryption) and the older version 1 (0x181C, 0x181E),
// finds the service data in raw advertising data, and builds plain and
// encrypted version 2 packets for simulators and tests.
//
// It needs no Bluetooth stack: the advertising bytes come from the
// application's own scanner, or from a Shelly Gen2+ device relaying what
// it hears (see shelly.ParseBLEScanResult). Standard library only; AES-CCM
// is implemented here because the standard library has none.
//
// The format is described at https://bthome.io; the object table here
// was written from that description and checked against bthome-ble
// 3.24.0. Not affiliated with the BTHome project.
package bthome

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

// Service UUIDs (16-bit).
const (
	UUID            uint16 = 0xFCD2 // version 2
	UUIDv1          uint16 = 0x181C // version 1, not encrypted
	UUIDv1Encrypted uint16 = 0x181E // version 1, encrypted
)

// Bits of the version 2 device information byte.
const (
	FlagEncrypted  = 0x01 // the objects are encrypted
	FlagMACPresent = 0x02 // the sender's address follows the information byte
	FlagTrigger    = 0x04 // the device sends at irregular times, on events
	version2       = 2 << 5
)

var (
	// ErrNotBTHome means the advertising data carries no BTHome service
	// data.
	ErrNotBTHome = errors.New("bthome: no BTHome service data")
	// ErrNoKey means the packet is encrypted and no key was given.
	ErrNoKey = errors.New("bthome: encrypted packet and no key")
	// ErrTruncated means an object runs past the end of the packet.
	ErrTruncated = errors.New("bthome: truncated packet")
)

// Object is one reading or event of a packet.
type Object struct {
	ID    byte
	Info  ObjectInfo
	Index int    // which occurrence of this object id in the packet, from 0
	Data  []byte // the value's bytes as sent (for a command: opcode then arguments)

	Value float64   // Sensor; Binary (0 or 1); Event (its code); Time (Unix seconds); DeviceInfo (type id)
	Text  string    // Text; Raw as lower-case hexadecimal; DeviceInfo firmware as "a.b.c(.d)"
	Event string    // Event: "press", "long_press", "rotate_left", "on", ...; "" for no event
	Steps int       // dimmer events: the steps turned
	Args  []byte    // command events: the arguments
	Time  time.Time // Time
}

// Bool reports a Binary object's state.
func (o Object) Bool() bool { return o.Value != 0 }

// Packet is one decoded advertisement.
type Packet struct {
	Version   int    // 1 or 2
	Encrypted bool   // it was encrypted (and was decrypted)
	Trigger   bool   // the device sends on events rather than at a regular interval
	MAC       string // the address carried in the payload (upper case, colons), or ""
	Counter   uint32 // encrypted packets: the sender's counter
	Objects   []Object
	// Rest holds the bytes from the first object id this package does not
	// know; the format asks receivers to stop there, since lengths of
	// unknown objects cannot be told.
	Rest []byte
}

// Find returns the index-th occurrence (from 0) of object id.
func (p *Packet) Find(id byte, index int) (Object, bool) {
	for _, o := range p.Objects {
		if o.ID == id && o.Index == index {
			return o, true
		}
	}
	return Object{}, false
}

// PacketID returns the packet id object (0x00), which senders count up so
// that receivers can drop repeats.
func (p *Packet) PacketID() (int, bool) {
	o, ok := p.Find(0x00, 0)
	return int(o.Value), ok
}

// ParseMAC reads an address written as "AA:BB:CC:DD:EE:FF",
// "aa-bb-cc-dd-ee-ff" or "AABBCCDDEEFF".
func ParseMAC(s string) ([6]byte, error) {
	var m [6]byte
	h := strings.NewReplacer(":", "", "-", "", ".", "").Replace(s)
	b, err := hex.DecodeString(h)
	if err != nil || len(b) != 6 {
		return m, fmt.Errorf("bthome: bad address %q", s)
	}
	copy(m[:], b)
	return m, nil
}

// FormatMAC writes an address as "AA:BB:CC:DD:EE:FF".
func FormatMAC(m [6]byte) string {
	parts := make([]string, 6)
	for i, x := range m {
		parts[i] = fmt.Sprintf("%02X", x)
	}
	return strings.Join(parts, ":")
}

// Decode reads the service data of one of the BTHome UUIDs (the bytes
// after the UUID). addr is the sender's Bluetooth address; it is needed
// only for an encrypted packet that does not carry its address. key is
// the 16-byte bind key of an encrypted sender, or nil.
func Decode(uuid uint16, data []byte, addr string, key []byte) (*Packet, error) {
	switch uuid {
	case UUID:
		return decodeV2(data, addr, key)
	case UUIDv1, UUIDv1Encrypted:
		return decodeV1(uuid == UUIDv1Encrypted, data, addr, key)
	}
	return nil, ErrNotBTHome
}

// DecodeAdvertising finds BTHome service data in raw advertising data
// (the advertisement and the scan response, concatenated) and decodes it.
func DecodeAdvertising(adv []byte, addr string, key []byte) (*Packet, error) {
	ads, err := ParseAD(adv)
	if err != nil && len(ads) == 0 {
		return nil, err
	}
	sd := ServiceData16(ads)
	for _, u := range []uint16{UUID, UUIDv1Encrypted, UUIDv1} {
		if d, ok := sd[u]; ok {
			return Decode(u, d, addr, key)
		}
	}
	return nil, ErrNotBTHome
}

func decodeV2(data []byte, addr string, key []byte) (*Packet, error) {
	if len(data) < 1 {
		return nil, ErrTruncated
	}
	info := data[0]
	if v := int(info >> 5); v != 2 {
		return nil, fmt.Errorf("bthome: version bits say %d, not 2", v)
	}
	p := &Packet{Version: 2, Encrypted: info&FlagEncrypted != 0, Trigger: info&FlagTrigger != 0}
	body := data[1:]
	var mac [6]byte
	haveMAC := false
	if info&FlagMACPresent != 0 {
		if len(body) < 6 {
			return nil, ErrTruncated
		}
		for i := 0; i < 6; i++ { // sent least significant byte first
			mac[i] = body[5-i]
		}
		haveMAC = true
		p.MAC = FormatMAC(mac)
		body = body[6:]
	}
	if p.Encrypted {
		if key == nil {
			return p, ErrNoKey
		}
		if len(body) < 8 {
			return nil, ErrTruncated
		}
		if !haveMAC {
			m, err := ParseMAC(addr)
			if err != nil {
				return nil, err
			}
			mac = m
		}
		ct, ctr, tag := body[:len(body)-8], body[len(body)-8:len(body)-4], body[len(body)-4:]
		c, err := newCCM(key, 4)
		if err != nil {
			return nil, err
		}
		nonce := append(append(append(mac[:0:0], mac[:]...), 0xD2, 0xFC, info), ctr...)
		plain, err := c.open(nonce, ct, tag, nil)
		if err != nil {
			return nil, err
		}
		p.Counter = binary.LittleEndian.Uint32(ctr)
		body = plain
	}
	objs, rest, err := parseV2Objects(body)
	p.Objects, p.Rest = objs, rest
	return p, err
}

func parseV2Objects(body []byte) ([]Object, []byte, error) {
	var out []Object
	seen := map[byte]int{}
	for i := 0; i < len(body); {
		id := body[i]
		info, ok := table[id]
		if !ok {
			return out, body[i:], nil
		}
		start := i + 1
		var n int
		switch {
		case id == 0x3B: // command: a length byte holds the argument count
			if start >= len(body) {
				return out, nil, ErrTruncated
			}
			n = 1 + int(body[start]&0x1F)
			start++
		case info.Size == 0:
			if start >= len(body) {
				return out, nil, ErrTruncated
			}
			n = int(body[start])
			start++
		default:
			n = info.Size
		}
		if start+n > len(body) {
			return out, nil, ErrTruncated
		}
		o := Object{ID: id, Info: info, Index: seen[id], Data: append([]byte(nil), body[start:start+n]...)}
		seen[id]++
		fill(&o)
		out = append(out, o)
		i = start + n
	}
	return out, nil, nil
}

// fill sets the value fields of o from its bytes and its information.
func fill(o *Object) {
	d := o.Data
	switch o.Info.Kind {
	case Sensor, Binary:
		o.Value = scaled(intOf(d, o.Info.Signed), o.Info.Factor)
	case Time:
		o.Value = float64(uintOf(d))
		o.Time = time.Unix(int64(uintOf(d)), 0).UTC()
	case Text:
		o.Text = string(d)
	case Raw:
		o.Text = hex.EncodeToString(d)
	case DeviceInfo:
		switch o.ID {
		case 0xF0:
			o.Value = float64(uintOf(d))
		default: // firmware: least significant part first
			parts := make([]string, len(d))
			for i := range d {
				parts[i] = fmt.Sprint(d[len(d)-1-i])
			}
			o.Text = strings.Join(parts, ".")
		}
	case Event:
		if len(d) == 0 {
			return
		}
		o.Value = float64(d[0])
		switch o.ID {
		case 0x3A:
			o.Event = buttonEvents[d[0]]
		case 0x3C:
			o.Event = dimmerEvents[d[0]]
			if len(d) > 1 {
				o.Steps = int(d[1])
			}
		case 0x3B:
			o.Event = commandEvents[d[0]]
			o.Args = append([]byte(nil), d[1:]...)
		}
	}
}

func uintOf(d []byte) uint64 {
	var v uint64
	for i := len(d) - 1; i >= 0; i-- {
		v = v<<8 | uint64(d[i])
	}
	return v
}

func intOf(d []byte, signed bool) float64 {
	v := uintOf(d)
	if signed && len(d) > 0 && len(d) < 8 && d[len(d)-1]&0x80 != 0 {
		return float64(int64(v) - int64(1)<<(8*uint(len(d))))
	}
	return float64(v)
}

// scaled multiplies and rounds away the binary noise of the factor.
func scaled(v, factor float64) float64 {
	if factor == 1 {
		return v
	}
	digits := 0
	for f := factor; f < 1 && digits < 9; f *= 10 {
		digits++
	}
	if factor == 0.35 {
		digits = 2
	}
	p := math.Pow(10, float64(digits))
	return math.Round(v*factor*p) / p
}

// ---- version 1 -------------------------------------------------------------------

func decodeV1(encrypted bool, data []byte, addr string, key []byte) (*Packet, error) {
	p := &Packet{Version: 1, Encrypted: encrypted}
	body := data
	if encrypted {
		if key == nil {
			return p, ErrNoKey
		}
		if len(body) < 8 {
			return nil, ErrTruncated
		}
		mac, err := ParseMAC(addr)
		if err != nil {
			return nil, err
		}
		ct, ctr, tag := body[:len(body)-8], body[len(body)-8:len(body)-4], body[len(body)-4:]
		c, err := newCCM(key, 4)
		if err != nil {
			return nil, err
		}
		nonce := append(append(append(mac[:0:0], mac[:]...), 0x1E, 0x18), ctr...)
		plain, err := c.open(nonce, ct, tag, []byte{0x11})
		if err != nil {
			return nil, err
		}
		p.Counter = binary.LittleEndian.Uint32(ctr)
		body = plain
	}
	seen := map[byte]int{}
	for i := 0; i < len(body); {
		if i+2 > len(body) {
			return p, ErrTruncated
		}
		ctl := body[i]
		size := int(ctl & 0x1F) // the type byte and the value
		format := ctl >> 5
		id := body[i+1]
		if size < 1 || i+1+size > len(body) {
			return p, ErrTruncated
		}
		d := append([]byte(nil), body[i+2:i+1+size]...)
		i += 1 + size
		info, ok := table[id]
		if !ok {
			info = ObjectInfo{ID: id, Name: "unknown", Kind: Raw, Factor: 1}
		}
		o := Object{ID: id, Info: info, Index: seen[id], Data: d}
		seen[id]++
		switch format {
		case 0, 1:
			o.Value = scaled(intOf(d, format == 1), info.Factor)
			if info.Kind == Event {
				fill(&o)
			}
		case 2:
			o.Value = scaled(floatOf(d), info.Factor)
		case 3:
			o.Text = string(d)
		case 4:
			o.Text = hex.EncodeToString(d)
		}
		p.Objects = append(p.Objects, o)
	}
	return p, nil
}

func floatOf(d []byte) float64 {
	switch len(d) {
	case 2:
		return halfFloat(binary.LittleEndian.Uint16(d))
	case 4:
		return float64(math.Float32frombits(binary.LittleEndian.Uint32(d)))
	case 8:
		return math.Float64frombits(binary.LittleEndian.Uint64(d))
	}
	return math.NaN()
}

func halfFloat(h uint16) float64 {
	sign := 1.0
	if h&0x8000 != 0 {
		sign = -1
	}
	exp := int(h>>10) & 0x1F
	frac := float64(h & 0x3FF)
	switch exp {
	case 0:
		return sign * frac / 1024 * math.Pow(2, -14)
	case 31:
		if frac == 0 {
			return math.Inf(int(sign))
		}
		return math.NaN()
	}
	return sign * (1 + frac/1024) * math.Pow(2, float64(exp-15))
}

// ---- replay guard ----------------------------------------------------------------

// Guard drops encrypted packets whose counter did not go up since the last
// accepted packet of the same sender, as the format asks receivers to.
// The zero value is ready to use and safe for concurrent use.
type Guard struct {
	mu   sync.Mutex
	last map[string]uint32
}

// Accept reports whether counter is newer than the last one accepted for
// mac, and remembers it when it is.
func (g *Guard) Accept(mac string, counter uint32) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.last == nil {
		g.last = map[string]uint32{}
	}
	k := strings.ToUpper(mac)
	if last, ok := g.last[k]; ok && counter <= last {
		return false
	}
	g.last[k] = counter
	return true
}

// Reset forgets mac's counter (after the sender's key and counter were
// reset).
func (g *Guard) Reset(mac string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.last, strings.ToUpper(mac))
}
