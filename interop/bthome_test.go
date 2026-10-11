// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package interop

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/cB-Sigma/shelly/bthome"
)

const (
	pyImage       = "python:3.12.10-slim"
	bthomeBLEPkg  = "bthome-ble==3.24.0"
	interopMAC    = "54:48:E6:8F:80:A5"
	interopKeyHex = "231d39c1d7cc1ab1aee224cd096db932"
	containerVecs = "/vec/vectors.json"
	containerPeer = "/sigma/bthome_peer.py"
)

type vector struct {
	MAC  string `json:"mac"`
	UUID uint16 `json:"uuid"`
	SD   string `json:"sd"`
	Key  string `json:"key"`

	desc string
	want *bthome.Packet
}

type peerOut struct {
	Sensors map[string]any  `json:"sensors"`
	Binary  map[string]bool `json:"binary"`
	Events  []struct {
		Key   string         `json:"key"`
		Type  string         `json:"type"`
		Props map[string]any `json:"props"`
	} `json:"events"`
	Verified bool   `json:"bindkey_verified"`
	Error    string `json:"error"`
}

// rawSample is an integer that fits an object of size bytes.
func rawSample(size int, signed bool) float64 {
	switch {
	case size == 1 && signed:
		return -50
	case size == 1:
		return 200
	case size == 2 && signed:
		return -1234
	case size == 2:
		return 4660
	case size == 3:
		return 70000
	case signed:
		return -3000000
	}
	return 3000000
}

func buildVectors(t *testing.T) []vector {
	t.Helper()
	key, _ := hex.DecodeString(interopKeyHex)
	var out []vector
	add := func(desc string, b *bthome.Builder, seal bool) {
		var sd []byte
		var err error
		k := ""
		if seal {
			sd, err = b.Seal(key, interopMAC, uint32(len(out)+1))
			k = interopKeyHex
		} else {
			sd, err = b.ServiceData()
		}
		if err != nil {
			t.Fatalf("%s: %v", desc, err)
		}
		p, err := bthome.Decode(bthome.UUID, sd, interopMAC, key)
		if err != nil {
			t.Fatalf("%s: own decode: %v", desc, err)
		}
		out = append(out, vector{MAC: interopMAC, UUID: bthome.UUID, SD: hex.EncodeToString(sd), Key: k, desc: desc, want: p})
	}
	for _, o := range bthome.Objects() {
		var b bthome.Builder
		switch o.Kind {
		case bthome.Sensor:
			b.Add(o.ID, rawSample(o.Size, o.Signed)*o.Factor)
		case bthome.Binary:
			b.Add(o.ID, 1)
		case bthome.Time:
			b.Add(o.ID, 1684093277)
		case bthome.Text:
			b.Text("Hello World!")
		case bthome.Raw:
			b.Raw([]byte{0xAA, 0xBB, 0xCC})
		default:
			continue
		}
		add(fmt.Sprintf("0x%02X %s", o.ID, o.Name), &b, false)
	}
	for _, name := range []string{"press", "double_press", "triple_press", "long_press", "long_double_press", "long_triple_press", "hold_press"} {
		c, _ := bthome.ButtonEvent(name)
		var b bthome.Builder
		add("button "+name, b.Button(c), false)
	}
	for _, name := range []string{"rotate_left", "rotate_right"} {
		c, _ := bthome.DimmerEvent(name)
		var b bthome.Builder
		add("dimmer "+name, b.Dimmer(c, 3), false)
	}
	for _, name := range []string{"off", "on", "toggle", "step_up", "step_down"} {
		c, _ := bthome.CommandEvent(name)
		var b bthome.Builder
		add("command "+name, b.Command(c, 5), false)
	}
	var enc bthome.Builder
	enc.Add(0x00, 17).Add(0x01, 91).Add(0x02, -7.25).Add(0x03, 48.5).Add(0x2D, 1).Button(1)
	add("encrypted, several objects", &enc, true)
	withMAC := bthome.Builder{MAC: interopMAC, Trigger: true}
	withMAC.Add(0x05, 140.5).Add(0x21, 1)
	add("encrypted, address in the payload, trigger flag", &withMAC, true)
	return out
}

func near(a, b float64) bool {
	return math.Abs(a-b) <= 1e-6*math.Max(1, math.Abs(a))
}

// Every object id package bthome knows, built by package bthome and decoded
// by bthome-ble: the values must agree.
func TestBTHomeAgainstBthomeBLE(t *testing.T) {
	if os.Getenv("SIGMA_SHELLY_INTEROP") != "1" {
		t.Skip("runs bthome-ble in Docker; set SIGMA_SHELLY_INTEROP=1 to run")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not on PATH")
	}
	vecs := buildVectors(t)
	dir := t.TempDir()
	b, _ := json.Marshal(vecs)
	if err := os.WriteFile(filepath.Join(dir, "vectors.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	td, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("sigma-shelly-bthome-%d", os.Getpid())
	out, err := exec.Command("docker", "run", "-d", "--name", name, "-v", td+":/sigma:ro", "-v", dir+":/vec:ro", pyImage, "sleep", "1800").CombinedOutput()
	if err != nil {
		t.Fatalf("docker run: %v: %s", err, out)
	}
	defer exec.Command("docker", "container", "rm", "-f", name).Run()
	if out, err := exec.Command("docker", "exec", name, "pip", "install", "--no-cache-dir", "--disable-pip-version-check",
		"--root-user-action=ignore", bthomeBLEPkg).CombinedOutput(); err != nil {
		t.Fatalf("pip install: %v: %s", err, out)
	}
	res, err := exec.Command("docker", "exec", name, "python", "-I", containerPeer, containerVecs).Output()
	if err != nil {
		t.Fatalf("peer: %v", err)
	}
	sc := bufio.NewScanner(bytes.NewReader(res))
	sc.Buffer(nil, 1<<20)
	i := 0
	for ; sc.Scan(); i++ {
		if i >= len(vecs) {
			t.Fatalf("more answers than vectors")
		}
		v := vecs[i]
		var po peerOut
		if err := json.Unmarshal(sc.Bytes(), &po); err != nil {
			t.Fatalf("%s: %v: %s", v.desc, err, sc.Bytes())
		}
		if po.Error != "" {
			t.Errorf("FAIL %s: bthome-ble: %s", v.desc, po.Error)
			continue
		}
		if v.Key != "" && !po.Verified {
			t.Errorf("FAIL %s: bthome-ble did not verify the key", v.desc)
		}
		if msg := compare(v.want, po); msg != "" {
			t.Errorf("FAIL %s (%s): %s; bthome-ble: %s", v.desc, v.SD, msg, sc.Bytes())
		} else {
			t.Logf("PASS %s", v.desc)
		}
	}
	if i != len(vecs) {
		t.Fatalf("%d answers for %d vectors", i, len(vecs))
	}
}

// compare checks that bthome-ble found the same values as package bthome.
// bthome-ble names values by Home Assistant device class, so values are
// matched by kind and value rather than by name.
func compare(p *bthome.Packet, po peerOut) string {
	nums := map[float64]int{}
	for _, v := range po.Sensors {
		switch x := v.(type) {
		case float64:
			nums[x]++
		}
	}
	strs := map[string]bool{}
	for _, v := range po.Sensors {
		if s, ok := v.(string); ok {
			strs[s] = true
		}
	}
	trues := 0
	for _, v := range po.Binary {
		if v {
			trues++
		}
	}
	wantTrue := 0
	for _, o := range p.Objects {
		switch o.Info.Kind {
		case bthome.Sensor, bthome.Time:
			found := false
			for n := range nums {
				if near(n, o.Value) {
					found = true
				}
			}
			if !found {
				return fmt.Sprintf("0x%02X %s = %v not among bthome-ble's values", o.ID, o.Info.Name, o.Value)
			}
		case bthome.Binary:
			if o.Bool() {
				wantTrue++
			}
		case bthome.Text, bthome.Raw:
			if !strs[o.Text] {
				return fmt.Sprintf("%s %q not found", o.Info.Name, o.Text)
			}
		case bthome.Event:
			found := false
			for _, e := range po.Events {
				if e.Type != o.Event {
					continue
				}
				switch o.ID {
				case 0x3C:
					found = found || e.Props["steps"] == float64(o.Steps)
				case 0x3B:
					found = found || e.Props["args"] == hex.EncodeToString(o.Args)
				default:
					found = true
				}
			}
			if !found {
				return fmt.Sprintf("event %s %q (steps %d, args %x) not found", o.Info.Name, o.Event, o.Steps, o.Args)
			}
		}
	}
	if wantTrue != trues {
		return fmt.Sprintf("%d binary sensors on, bthome-ble %d", wantTrue, trues)
	}
	return ""
}
