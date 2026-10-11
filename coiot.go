package shelly

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// CoIoT is how Gen1 Shelly devices push their state, as Shelly's CoIoT
// document describes it: CoAP (RFC 7252) non-confirmable messages to the
// multicast group 224.0.1.187 port 5683 (or to a "CoIoT peer" address
// set on the device), sent on every change and periodically. A status
// message has the URI path /cit/s and the code 0.30; option 3332 names
// the device ("<type>#<id>#<CoIoT version>"), option 3412 how long the
// values stay valid, option 3420 a serial that changes with the state.
// The payload is {"G": [[channel, sensor ID, value], …]}; what each
// sensor ID is comes from the device's description, GET /cit/d (blocks
// with "I" and "D", sensors with "I", type "T", description "D", unit
// "U" and the block "L" they belong to).
//
// ListenCoIoT receives these messages, ParseCoAP reads one, and
// CitDesc.Decode turns its payload into values with the device's
// description. A sleeping device's values can only be taken from its
// messages: it is asleep again before it could be asked.

// CoIoT defaults: the multicast group and UDP port devices send to.
const (
	CoIoTGroup = "224.0.1.187"
	CoIoTPort  = 5683
)

// CoAP option numbers Shelly uses.
const (
	optURIPath  = 11
	optDeviceID = 3332
	optValidity = 3412
	optSerial   = 3420
)

// CoIoTMessage is one received CoIoT message.
type CoIoTMessage struct {
	// Code is the CoAP code (30, that is 0.30, for a status message).
	Code byte
	// Path is the URI path (/cit/s).
	Path string
	// DevType is the device's type (SHSW-25).
	DevType string
	// DevID is its id, the MAC's hexadecimal digits (A4CF12F45B2E).
	DevID string
	// Version is the CoIoT version the device names.
	Version string
	// Serial changes when the device's state changes.
	Serial int
	// Payload is the JSON status ({"G": [...]}).
	Payload []byte
}

// ParseCoAP reads a CoAP message as CoIoT carries it.
func ParseCoAP(b []byte) (CoIoTMessage, error) {
	var m CoIoTMessage
	if len(b) < 4 || b[0]>>6 != 1 {
		return m, errors.New("not a CoAP version 1 message")
	}
	tkl := int(b[0] & 0x0f)
	m.Code = b[1]
	i := 4 + tkl
	if i > len(b) {
		return m, errors.New("CoAP token runs past the message")
	}
	opt := 0
	var path []string
	for i < len(b) && b[i] != 0xff {
		delta, length := int(b[i]>>4), int(b[i]&0x0f)
		i++
		ext := func(v int) (int, error) {
			switch v {
			case 13:
				if i >= len(b) {
					return 0, errors.New("short option")
				}
				i++
				return int(b[i-1]) + 13, nil
			case 14:
				if i+1 >= len(b) {
					return 0, errors.New("short option")
				}
				i += 2
				return int(binary.BigEndian.Uint16(b[i-2:])) + 269, nil
			case 15:
				return 0, errors.New("reserved option nibble")
			}
			return v, nil
		}
		var err error
		if delta, err = ext(delta); err != nil {
			return m, err
		}
		if length, err = ext(length); err != nil {
			return m, err
		}
		opt += delta
		if i+length > len(b) {
			return m, errors.New("CoAP option runs past the message")
		}
		val := b[i : i+length]
		i += length
		switch opt {
		case optURIPath:
			path = append(path, string(val))
		case optDeviceID:
			p := strings.Split(string(val), "#")
			m.DevType = p[0]
			if len(p) > 1 {
				m.DevID = p[1]
			}
			if len(p) > 2 {
				m.Version = p[2]
			}
		case optSerial:
			for _, x := range val {
				m.Serial = m.Serial<<8 | int(x)
			}
		}
	}
	if i < len(b) && b[i] == 0xff {
		m.Payload = b[i+1:]
	}
	m.Path = "/" + strings.Join(path, "/")
	return m, nil
}

// EncodeCoIoT builds a CoIoT status message to /cit/s with serial and
// payload, as a device sends it (for simulators and tests).
func EncodeCoIoT(devType, devID string, serial int, payload []byte) []byte {
	b := []byte{0x50, 30, 0, 1} // version 1, NON, no token; code 0.30
	last := 0
	put := func(num int, val []byte) {
		delta := num - last
		last = num
		nib := func(v int) (byte, []byte) {
			switch {
			case v < 13:
				return byte(v), nil
			case v < 269:
				return 13, []byte{byte(v - 13)}
			}
			e := make([]byte, 2)
			binary.BigEndian.PutUint16(e, uint16(v-269))
			return 14, e
		}
		dn, de := nib(delta)
		ln, le := nib(len(val))
		b = append(b, dn<<4|ln)
		b = append(b, de...)
		b = append(b, le...)
		b = append(b, val...)
	}
	put(optURIPath, []byte("cit"))
	put(optURIPath, []byte("s"))
	put(optDeviceID, []byte(devType+"#"+devID+"#2"))
	put(optValidity, []byte{0x75, 0x30})
	put(optSerial, []byte{byte(serial >> 8), byte(serial)})
	b = append(b, 0xff)
	return append(b, payload...)
}

// CitSensor is one sensor of a /cit/d description.
type CitSensor struct {
	// I is the sensor ID the status triples refer to.
	I int `json:"I"`
	// T is the sensor type (S state, P power, E energy, T temperature,
	// EV event, EVC event count, …).
	T string `json:"T"`
	// D is the description ("output", "power", "inputEvent", …).
	D string `json:"D"`
	// U is the unit (W, Wmin, C, %), when it has one.
	U string `json:"U"`
	// L is the ID of the block the sensor belongs to; for a sensor the
	// description gives several blocks, the first of them.
	L int `json:"L"`
	// Blocks are all the block IDs of a sensor whose L is a list in the
	// description; nil when L is a single number.
	Blocks []int `json:"-"`
}

// UnmarshalJSON reads a sensor whose L is a block ID or a list of them.
func (s *CitSensor) UnmarshalJSON(b []byte) error {
	var raw struct {
		I int             `json:"I"`
		T string          `json:"T"`
		D string          `json:"D"`
		U string          `json:"U"`
		L json.RawMessage `json:"L"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*s = CitSensor{I: raw.I, T: raw.T, D: raw.D, U: raw.U}
	l := bytes.TrimSpace(raw.L)
	switch {
	case len(l) == 0 || string(l) == "null":
	case l[0] == '[':
		if err := json.Unmarshal(l, &s.Blocks); err != nil {
			return fmt.Errorf("cit/d sensor %d: L: %w", raw.I, err)
		}
		if len(s.Blocks) > 0 {
			s.L = s.Blocks[0]
		}
	default:
		if err := json.Unmarshal(l, &s.L); err != nil {
			return fmt.Errorf("cit/d sensor %d: L: %w", raw.I, err)
		}
	}
	return nil
}

// CitBlock is one block of a /cit/d description: a part of the device
// ("relay_0", "input_1", "device").
type CitBlock struct {
	// I is the block ID sensors name in L.
	I int `json:"I"`
	// D is the block's description.
	D string `json:"D"`
}

// CitDesc is a device's /cit/d description.
type CitDesc struct {
	// Blk are the device's blocks.
	Blk []CitBlock `json:"blk"`
	// Sen are its sensors.
	Sen []CitSensor `json:"sen"`
}

// BlockIndex is the index of a sensor's block among the device's blocks of
// the same kind, from the number after the last "_" of the block's
// description (relay_0 → 0, relay_1 → 1); ok is false when the block
// has none (the "device" block).
func (c CitDesc) BlockIndex(s CitSensor) (n int, ok bool) {
	name := ""
	for _, b := range c.Blk {
		if b.I == s.L {
			name = b.D
		}
	}
	if i := strings.LastIndex(name, "_"); i > 0 {
		if n, err := strconv.Atoi(name[i+1:]); err == nil {
			return n, true
		}
	}
	return 0, false
}

// CitValue is one value of a status message.
type CitValue struct {
	// Sensor is the sensor it belongs to.
	Sensor CitSensor
	// Index is the sensor's block index (BlockIndex); HasIndex is false
	// for a block without one.
	Index    int
	HasIndex bool
	// Value is the value as sent: a float64, a string, or another JSON
	// value.
	Value any
}

// Decode reads a status payload ({"G": [[channel, sensor ID, value], …]})
// with the description, in the payload's order. Triples of sensors the
// description does not list are left out; a payload that is not JSON
// gives nothing.
func (c CitDesc) Decode(payload []byte) []CitValue {
	var st struct {
		G [][]any `json:"G"`
	}
	if json.Unmarshal(payload, &st) != nil {
		return nil
	}
	byID := map[int]CitSensor{}
	for _, s := range c.Sen {
		byID[s.I] = s
	}
	var out []CitValue
	for _, g := range st.G {
		if len(g) < 3 {
			continue
		}
		idf, _ := g[1].(float64)
		s, ok := byID[int(idf)]
		if !ok {
			continue
		}
		n, has := c.BlockIndex(s)
		out = append(out, CitValue{Sensor: s, Index: n, HasIndex: has, Value: g[2]})
	}
	return out
}

// CitDescription reads a Gen1 device's CoIoT description (GET /cit/d).
func (c *Client) CitDescription(ctx context.Context) (CitDesc, error) {
	var desc CitDesc
	raw, err := c.get(ctx, "/cit/d", true)
	if err != nil {
		return desc, err
	}
	if err := json.Unmarshal(raw, &desc); err != nil {
		return desc, err
	}
	return desc, nil
}

// Gen1Event names a Gen1 input event the way Gen2 devices name theirs:
// S single_push, L long_push, SS double_push, SSS triple_push, SL
// short_long (other codes as sent, lower case).
func Gen1Event(e string) string {
	switch strings.ToUpper(e) {
	case "S":
		return "single_push"
	case "L":
		return "long_push"
	case "SS":
		return "double_push"
	case "SSS":
		return "triple_push"
	case "SL":
		return "short_long"
	case "LS":
		return "long_short"
	}
	return strings.ToLower(e)
}

// CoIoTListener receives CoIoT messages on UDP.
type CoIoTListener struct {
	c *net.UDPConn
}

// ListenCoIoT opens a CoIoT listener on addr (":5683" when nil). With a
// group ("" means CoIoTGroup) it joins that multicast group on addr's
// port; "none" listens for unicast only, for devices given this host as
// their CoIoT peer. When the group cannot be joined it falls back to
// unicast on addr.
func ListenCoIoT(addr *net.UDPAddr, group string) (*CoIoTListener, error) {
	if addr == nil {
		addr = &net.UDPAddr{Port: CoIoTPort}
	}
	if group == "" {
		group = CoIoTGroup
	}
	var c *net.UDPConn
	var err error
	if group != "none" {
		gaddr := &net.UDPAddr{IP: net.ParseIP(group), Port: addr.Port}
		c, err = net.ListenMulticastUDP("udp4", nil, gaddr)
	}
	if c == nil {
		// Unicast only (CoIoT peer), or multicast could not be joined.
		c, err = net.ListenUDP("udp4", addr)
	}
	if err != nil {
		return nil, err
	}
	return &CoIoTListener{c: c}, nil
}

// Addr is the address the listener is bound to.
func (l *CoIoTListener) Addr() string { return l.c.LocalAddr().String() }

// Close stops the listener.
func (l *CoIoTListener) Close() error { return l.c.Close() }

// Serve calls fn for each status message (a /cit/s path and a device id)
// with its sender until ctx ends or the listener is closed; it closes the
// listener when it returns.
func (l *CoIoTListener) Serve(ctx context.Context, fn func(m CoIoTMessage, from *net.UDPAddr)) error {
	stop := context.AfterFunc(ctx, func() { _ = l.c.Close() })
	defer stop()
	defer l.c.Close()
	buf := make([]byte, 4096)
	for {
		n, from, err := l.c.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		m, err := ParseCoAP(buf[:n])
		if err != nil || m.DevID == "" || !strings.HasPrefix(m.Path, "/cit/s") {
			continue
		}
		fn(m, from)
	}
}
