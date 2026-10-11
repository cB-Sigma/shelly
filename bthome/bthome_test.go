// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package bthome

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"
	"time"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// RFC 3610, packet vector #1: an 8-byte tag with additional data, which
// exercises the parts of CCM that BTHome version 1 uses.
func TestCCMRFC3610(t *testing.T) {
	key := unhex(t, "C0C1C2C3C4C5C6C7C8C9CACBCCCDCECF")
	nonce := unhex(t, "00000003020100A0A1A2A3A4A5")
	aad := unhex(t, "0001020304050607")
	msg := unhex(t, "08090A0B0C0D0E0F101112131415161718191A1B1C1D1E")
	c, err := newCCM(key, 8)
	if err != nil {
		t.Fatal(err)
	}
	ct, tag, err := c.seal(nonce, msg, aad)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := hex.EncodeToString(append(ct, tag...)), "588c979a61c663d2f066d0c2c0f989806d5f6b61dac38417e8d12cfdf926e0"; got != want {
		t.Fatalf("seal = %s, want %s", got, want)
	}
	back, err := c.open(nonce, ct, tag, aad)
	if err != nil || !bytes.Equal(back, msg) {
		t.Fatalf("open = %x, %v", back, err)
	}
	tag[0] ^= 1
	if _, err := c.open(nonce, ct, tag, aad); !errors.Is(err, ErrAuth) {
		t.Fatalf("a changed tag gave %v", err)
	}
}

// The example of the format's description: temperature and humidity.
func TestDecodePlain(t *testing.T) {
	p, err := Decode(UUID, unhex(t, "4002C40903BF13"), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != 2 || p.Encrypted || p.Trigger || len(p.Objects) != 2 || p.Rest != nil {
		t.Fatalf("packet = %+v", p)
	}
	if o := p.Objects[0]; o.Info.Name != "temperature" || o.Value != 25 || o.Info.Unit != "°C" {
		t.Errorf("temperature = %+v", o)
	}
	if o := p.Objects[1]; o.Info.Name != "humidity" || o.Value != 50.55 {
		t.Errorf("humidity = %+v", o)
	}
}

// The encryption example of the format's description, both ways.
func TestEncryptedExample(t *testing.T) {
	key := unhex(t, "231d39c1d7cc1ab1aee224cd096db932")
	const mac = "54:48:E6:8F:80:A5"
	sd := unhex(t, "41e445f3c9962b332211006c7c4519")
	p, err := Decode(UUID, sd, mac, key)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Encrypted || p.Counter != 1122867 || len(p.Objects) != 2 || p.Objects[0].Value != 25.06 || p.Objects[1].Value != 50.55 {
		t.Fatalf("packet = %+v", p)
	}
	var b Builder
	got, err := b.Add(0x03, 50.55).Add(0x02, 25.06).Seal(key, mac, 1122867)
	if err != nil || !bytes.Equal(got, sd) {
		t.Fatalf("Seal = %x, %v; want %x", got, err, sd)
	}
	if _, err := Decode(UUID, sd, "54:48:E6:8F:80:A6", key); !errors.Is(err, ErrAuth) {
		t.Errorf("wrong address: %v", err)
	}
	if _, err := Decode(UUID, sd, mac, nil); !errors.Is(err, ErrNoKey) {
		t.Errorf("no key: %v", err)
	}
}

// Values from the examples of the format's object table.
func TestObjects(t *testing.T) {
	for _, c := range []struct {
		hex   string
		name  string
		value float64
		text  string
		event string
	}{
		{"04138A01", "pressure", 1008.83, "", ""},
		{"05138A14", "illuminance", 13460.67, "", ""},
		{"3F020C", "rotation", 307.4, "", ""},
		{"518756", "acceleration", 22.151, "", ""},
		{"630057D0FF", "acceleration", -3.123456, "", ""},
		{"2D01", "window", 1, "", ""},
		{"0164", "battery", 100, "", ""},
		{"451101", "temperature", 27.3, "", ""},
		{"530C48656C6C6F20576F726C6421", "text", 0, "Hello World!", ""},
		{"5404AABBCCDD", "raw", 0, "aabbccdd", ""},
		{"3A04", "button", 4, "", "long_press"},
		{"3A80", "button", 0x80, "", "hold_press"},
		{"3C0103", "dimmer", 1, "", "rotate_left"},
		{"3B010305", "command", 3, "", "step_up"},
		{"F2000106", "firmware_version", 0, "6.1.0", ""},
	} {
		p, err := Decode(UUID, unhex(t, "40"+c.hex), "", nil)
		if err != nil {
			t.Errorf("%s: %v", c.hex, err)
			continue
		}
		if len(p.Objects) == 0 {
			t.Errorf("%s: no objects (rest %x)", c.hex, p.Rest)
			continue
		}
		o := p.Objects[0]
		if o.Info.Name != c.name || o.Value != c.value || o.Text != c.text || o.Event != c.event {
			t.Errorf("%s: %+v", c.hex, o)
		}
	}
	p, _ := Decode(UUID, unhex(t, "403C0103"), "", nil)
	if p.Objects[0].Steps != 3 {
		t.Errorf("dimmer steps = %d", p.Objects[0].Steps)
	}
	p, _ = Decode(UUID, unhex(t, "403B010305"), "", nil)
	if !bytes.Equal(p.Objects[0].Args, []byte{5}) {
		t.Errorf("command args = %x", p.Objects[0].Args)
	}
	p, _ = Decode(UUID, unhex(t, "40505d396164"), "", nil)
	if !p.Objects[0].Time.Equal(time.Unix(0x6461395d, 0)) {
		t.Errorf("time = %v", p.Objects[0].Time)
	}
}

// A Shelly BLU button-style packet: packet id, battery, two buttons (the
// second index), and an unknown id that ends the parse.
func TestIndexAndRest(t *testing.T) {
	p, err := Decode(UUID, unhex(t, "44002001643A013A02FE0102"), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Trigger {
		t.Error("trigger flag not read")
	}
	if id, ok := p.PacketID(); !ok || id != 0x20 {
		t.Errorf("packet id %d %v", id, ok)
	}
	if o, ok := p.Find(0x3A, 1); !ok || o.Event != "double_press" {
		t.Errorf("second button = %+v", o)
	}
	if !bytes.Equal(p.Rest, []byte{0xFE, 0x01, 0x02}) {
		t.Errorf("rest = %x", p.Rest)
	}
	if _, err := Decode(UUID, unhex(t, "4002C4"), "", nil); !errors.Is(err, ErrTruncated) {
		t.Errorf("truncated: %v", err)
	}
	if _, err := Decode(UUID, unhex(t, "20"), "", nil); err == nil {
		t.Error("version 1 bits under the version 2 UUID accepted")
	}
}

func TestBuilderRoundTrip(t *testing.T) {
	key := unhex(t, "00112233445566778899aabbccddeeff")
	b := Builder{Trigger: true, MAC: "3C:2E:F5:71:D5:2A"}
	b.Add(0x00, 7).Add(0x01, 88).Add(0x2D, 1).Add(0x3F, -12.5).Add(0x05, 120.5).Button(1).Dimmer(2, 4).Text("hi").Firmware(1, 0, 21)
	for _, seal := range []bool{false, true} {
		var sd []byte
		var err error
		if seal {
			sd, err = b.Seal(key, "", 99)
		} else {
			sd, err = b.ServiceData()
		}
		if err != nil {
			t.Fatal(err)
		}
		adv := Advertising(sd, "SBBT-002C")
		p, err := DecodeAdvertising(adv, "", key)
		if err != nil {
			t.Fatalf("seal %v: %v", seal, err)
		}
		if p.MAC != "3C:2E:F5:71:D5:2A" || !p.Trigger || p.Encrypted != seal {
			t.Errorf("seal %v: header %+v", seal, p)
		}
		want := map[string]float64{"packet_id": 7, "battery": 88, "window": 1, "rotation": -12.5, "illuminance": 120.5}
		for _, o := range p.Objects {
			if v, ok := want[o.Info.Name]; ok && v != o.Value {
				t.Errorf("seal %v: %s = %v, want %v", seal, o.Info.Name, o.Value, v)
			}
		}
		var ids []byte
		for _, o := range p.Objects {
			ids = append(ids, o.ID)
		}
		if !bytes.Equal(ids, []byte{0x00, 0x01, 0x05, 0x2D, 0x3A, 0x3C, 0x3F, 0x53, 0xF2}) {
			t.Errorf("seal %v: object order %x", seal, ids)
		}
		ads, _ := ParseAD(adv)
		if LocalName(ads) != "SBBT-002C" {
			t.Errorf("name %q", LocalName(ads))
		}
	}
	var bad Builder
	if _, err := bad.Add(0x01, 300).ServiceData(); err == nil {
		t.Error("battery 300% accepted")
	}
	if _, err := (&Builder{}).Add(0x3A, 1).ServiceData(); err == nil {
		t.Error("Add accepted an event object")
	}
}

func TestV1(t *testing.T) {
	// Temperature 25.00 as a signed integer (format 1, length 3) and
	// humidity 50.55 (format 0).
	p, err := Decode(UUIDv1, unhex(t, "2302C4090303BF13"), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Version != 1 || len(p.Objects) != 2 || p.Objects[0].Value != 25 || p.Objects[1].Value != 50.55 {
		t.Fatalf("v1 = %+v", p)
	}
	// The same, encrypted as version 1 does (additional data 0x11).
	key := unhex(t, "231d39c1d7cc1ab1aee224cd096db932")
	mac, _ := ParseMAC("5448E68F80A5")
	c, _ := newCCM(key, 4)
	ctr := []byte{1, 0, 0, 0}
	nonce := append(append(append([]byte(nil), mac[:]...), 0x1E, 0x18), ctr...)
	ct, tag, _ := c.seal(nonce, unhex(t, "2302C4090303BF13"), []byte{0x11})
	sd := append(append(append([]byte(nil), ct...), ctr...), tag...)
	p, err = Decode(UUIDv1Encrypted, sd, "54:48:E6:8F:80:A5", key)
	if err != nil || p.Counter != 1 || len(p.Objects) != 2 || p.Objects[1].Value != 50.55 {
		t.Fatalf("v1 encrypted = %+v, %v", p, err)
	}
}

func TestGuard(t *testing.T) {
	var g Guard
	if !g.Accept("aa:bb", 5) || g.Accept("AA:BB", 5) || g.Accept("aa:bb", 4) || !g.Accept("aa:bb", 6) {
		t.Fatal("guard order")
	}
	g.Reset("aa:bb")
	if !g.Accept("aa:bb", 1) {
		t.Fatal("reset not taken")
	}
}

func TestInfo(t *testing.T) {
	if len(Objects()) < 90 {
		t.Errorf("%d objects", len(Objects()))
	}
	if o, ok := Info(0x02); !ok || o.Kind != Sensor || !o.Signed || o.Size != 2 {
		t.Errorf("0x02 = %+v", o)
	}
	if c, ok := ButtonEvent("long_press"); !ok || c != 4 {
		t.Errorf("long_press = %d", c)
	}
	if _, ok := ButtonEvent(""); ok {
		t.Error("the empty event has a code")
	}
}
