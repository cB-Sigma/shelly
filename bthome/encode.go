// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package bthome

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
)

// Builder builds version 2 service data, for simulators, tests and
// devices written in Go. Objects are written in object id order, as the
// format asks, whatever order they are added in (objects with the same id
// keep their order). The zero value is ready to use.
type Builder struct {
	Trigger bool   // set the trigger-based flag
	MAC     string // when set, carry this address in the payload
	objs    []builtObj
	err     error
}

type builtObj struct {
	id   byte
	data []byte // with the length byte for variable-length objects
}

func (b *Builder) add(id byte, data []byte) *Builder {
	b.objs = append(b.objs, builtObj{id, data})
	return b
}

func (b *Builder) fail(err error) *Builder {
	if b.err == nil {
		b.err = err
	}
	return b
}

// Add writes a Sensor, Binary or Time object: v is the value in the
// object's unit (for Binary, 0 or 1; for Time, Unix seconds).
func (b *Builder) Add(id byte, v float64) *Builder {
	info, ok := table[id]
	if !ok {
		return b.fail(fmt.Errorf("bthome: unknown object id 0x%02X", id))
	}
	switch info.Kind {
	case Sensor, Binary, Time:
	default:
		return b.fail(fmt.Errorf("bthome: object 0x%02X (%s) is not a number", id, info.Name))
	}
	raw := math.Round(v / info.Factor)
	bits := 8 * uint(info.Size)
	lo, hi := 0.0, math.Pow(2, float64(bits))-1
	if info.Signed {
		lo, hi = -math.Pow(2, float64(bits-1)), math.Pow(2, float64(bits-1))-1
	}
	if raw < lo || raw > hi || math.IsNaN(raw) {
		return b.fail(fmt.Errorf("bthome: %v does not fit object 0x%02X (%s)", v, id, info.Name))
	}
	u := uint64(int64(raw))
	data := make([]byte, info.Size)
	for i := range data {
		data[i] = byte(u >> (8 * uint(i)))
	}
	return b.add(id, data)
}

// Button writes a button event (ButtonEvent gives the codes; 0 is "no
// event", which a multi-button device sends for the buttons not pressed).
func (b *Builder) Button(code byte) *Builder { return b.add(0x3A, []byte{code}) }

// Dimmer writes a dimmer event with the steps turned.
func (b *Builder) Dimmer(code byte, steps uint8) *Builder { return b.add(0x3C, []byte{code, steps}) }

// Command writes a command event with up to 31 argument bytes.
func (b *Builder) Command(code byte, args ...byte) *Builder {
	if len(args) > 0x1F {
		return b.fail(errors.New("bthome: more than 31 command argument bytes"))
	}
	return b.add(0x3B, append([]byte{byte(len(args)), code}, args...))
}

// Text writes a text object (up to 255 bytes of UTF-8).
func (b *Builder) Text(s string) *Builder { return b.bytes(0x53, []byte(s)) }

// Raw writes a raw object (up to 255 bytes).
func (b *Builder) Raw(p []byte) *Builder { return b.bytes(0x54, p) }

func (b *Builder) bytes(id byte, p []byte) *Builder {
	if len(p) > 255 {
		return b.fail(errors.New("bthome: more than 255 bytes in one object"))
	}
	return b.add(id, append([]byte{byte(len(p))}, p...))
}

// DeviceType writes the device type id object (0xF0).
func (b *Builder) DeviceType(id uint16) *Builder {
	return b.add(0xF0, binary.LittleEndian.AppendUint16(nil, id))
}

// Firmware writes a firmware version object: three parts (0xF2) or four
// (0xF1), most significant first, such as Firmware(1, 0, 21).
func (b *Builder) Firmware(parts ...byte) *Builder {
	id := byte(0xF2)
	switch len(parts) {
	case 3:
	case 4:
		id = 0xF1
	default:
		return b.fail(errors.New("bthome: a firmware version has three or four parts"))
	}
	data := make([]byte, len(parts))
	for i := range parts {
		data[i] = parts[len(parts)-1-i]
	}
	return b.add(id, data)
}

// payload is the objects in id order.
func (b *Builder) payload() ([]byte, error) {
	if b.err != nil {
		return nil, b.err
	}
	objs := append([]builtObj(nil), b.objs...)
	sort.SliceStable(objs, func(i, j int) bool { return objs[i].id < objs[j].id })
	var out []byte
	for _, o := range objs {
		out = append(append(out, o.id), o.data...)
	}
	return out, nil
}

func (b *Builder) head(encrypted bool) ([]byte, [6]byte, error) {
	info := byte(version2)
	if encrypted {
		info |= FlagEncrypted
	}
	if b.Trigger {
		info |= FlagTrigger
	}
	var mac [6]byte
	if b.MAC == "" {
		return []byte{info}, mac, nil
	}
	m, err := ParseMAC(b.MAC)
	if err != nil {
		return nil, mac, err
	}
	head := []byte{info | FlagMACPresent}
	for i := 5; i >= 0; i-- {
		head = append(head, m[i])
	}
	return head, m, nil
}

// ServiceData returns the plain service data (the bytes after the UUID).
func (b *Builder) ServiceData() ([]byte, error) {
	p, err := b.payload()
	if err != nil {
		return nil, err
	}
	head, _, err := b.head(false)
	if err != nil {
		return nil, err
	}
	return append(head, p...), nil
}

// Seal returns encrypted service data: the objects encrypted with key (16
// bytes) for the sender addr and counter. Use a counter that goes up with
// every packet; receivers drop repeats.
func (b *Builder) Seal(key []byte, addr string, counter uint32) ([]byte, error) {
	p, err := b.payload()
	if err != nil {
		return nil, err
	}
	head, mac, err := b.head(true)
	if err != nil {
		return nil, err
	}
	if b.MAC == "" {
		if mac, err = ParseMAC(addr); err != nil {
			return nil, err
		}
	}
	c, err := newCCM(key, 4)
	if err != nil {
		return nil, err
	}
	ctr := binary.LittleEndian.AppendUint32(nil, counter)
	nonce := append(append(append([]byte(nil), mac[:]...), 0xD2, 0xFC, head[0]), ctr...)
	ct, tag, err := c.seal(nonce, p, nil)
	if err != nil {
		return nil, err
	}
	out := append(append(append(head, ct...), ctr...), tag...)
	return out, nil
}

// ---- advertising data ------------------------------------------------------------

// AD is one advertising data structure: its type and its data.
type AD struct {
	Type byte
	Data []byte
}

// Advertising data types used here.
const (
	ADFlags          = 0x01
	ADShortName      = 0x08
	ADCompleteName   = 0x09
	ADServiceData16  = 0x16
	ADManufacturer   = 0xFF
	adFlagsGeneralLE = 0x06 // LE general discoverable, BR/EDR not supported
)

// ParseAD splits advertising data into its structures. It stops at a
// zero length (padding) and returns what it read with an error when a
// structure runs past the end.
func ParseAD(b []byte) ([]AD, error) {
	var out []AD
	for i := 0; i < len(b); {
		n := int(b[i])
		if n == 0 {
			break
		}
		if i+1+n > len(b) {
			return out, ErrTruncated
		}
		out = append(out, AD{Type: b[i+1], Data: b[i+2 : i+1+n]})
		i += 1 + n
	}
	return out, nil
}

// ServiceData16 collects the 16-bit UUID service data of ads by UUID.
func ServiceData16(ads []AD) map[uint16][]byte {
	out := map[uint16][]byte{}
	for _, a := range ads {
		if a.Type == ADServiceData16 && len(a.Data) >= 2 {
			out[binary.LittleEndian.Uint16(a.Data)] = a.Data[2:]
		}
	}
	return out
}

// Manufacturer collects the manufacturer-specific data of ads by company
// identifier.
func Manufacturer(ads []AD) map[uint16][]byte {
	out := map[uint16][]byte{}
	for _, a := range ads {
		if a.Type == ADManufacturer && len(a.Data) >= 2 {
			out[binary.LittleEndian.Uint16(a.Data)] = a.Data[2:]
		}
	}
	return out
}

// LocalName returns the complete or shortened local name in ads.
func LocalName(ads []AD) string {
	short := ""
	for _, a := range ads {
		switch a.Type {
		case ADCompleteName:
			return string(a.Data)
		case ADShortName:
			short = string(a.Data)
		}
	}
	return short
}

// Advertising builds advertising data around version 2 service data: the
// flags, the service data and, when name is not empty, the complete local
// name.
func Advertising(serviceData []byte, name string) []byte {
	out := []byte{2, ADFlags, adFlagsGeneralLE}
	out = append(out, byte(len(serviceData)+3), ADServiceData16, byte(UUID&0xFF), byte(UUID>>8))
	out = append(out, serviceData...)
	if name != "" {
		out = append(out, byte(len(name)+1), ADCompleteName)
		out = append(out, name...)
	}
	return out
}
