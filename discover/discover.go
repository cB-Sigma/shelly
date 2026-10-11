// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

// Package discover finds Shelly devices on a network in three ways, each
// confirmed with the device's GET /shelly:
//
//   - DNS-SD: Gen2+ devices announce _shelly._tcp (with gen, app and ver
//     in TXT) and _http._tcp; Gen1 devices announce _http._tcp with an
//     instance name starting with "shelly". MDNS browses both through
//     the application's mDNS browser (a Browser), so this package needs
//     no mDNS code of its own; github.com/cB-Sigma/mdns is one browser
//     (see the README for the five lines that connect it).
//   - CoIoT: Gen1 devices multicast their status every few seconds;
//     FromCoIoT turns a message received with shelly.ListenCoIoT into a
//     candidate.
//   - A scan: Scan asks every address of a prefix for GET /shelly, for
//     networks where multicast does not reach.
//
// It is a separate package so the shelly package stays small; it uses
// only the standard library and the shelly package.
package discover

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cB-Sigma/shelly"
)

// Service types Shelly devices announce.
const (
	ServiceShelly = "_shelly._tcp" // Gen2 and later
	ServiceHTTP   = "_http._tcp"   // every generation
)

// Service is one DNS-SD service instance as an mDNS browser found it.
type Service struct {
	Instance string
	Host     string
	Port     int
	IPs      []net.IP
	TXT      map[string]string
}

// Browser browses one DNS-SD service type ("_shelly._tcp") on the local
// link and returns what answered.
type Browser func(ctx context.Context, service string) ([]Service, error)

// Device is a Shelly device found on the network.
type Device struct {
	// Addr is host:port of its HTTP API.
	Addr string
	// ID is the Gen2+ id or, for Gen1, shelly<type>-<MAC> as announced
	// (or the MAC alone when only CoIoT saw it).
	ID string
	// Gen is the generation (1, 2, 3, 4); 0 while unknown.
	Gen int
	// Model is the model or Gen1 type when known; App is the Gen2+
	// application name; Ver the firmware.
	Model, App, Ver string
	// Via says how it was found: mdns, coiot or scan.
	Via string
	// Info is GET /shelly's answer when it was read.
	Info *shelly.Info
}

// FromService turns a DNS-SD answer into a candidate; ok is false when
// it is not a Shelly device.
func FromService(s Service) (Device, bool) {
	host := ""
	if len(s.IPs) > 0 {
		host = s.IPs[0].String()
	} else if s.Host != "" {
		host = strings.TrimSuffix(s.Host, ".")
	}
	if host == "" || s.Port == 0 {
		return Device{}, false
	}
	inst := strings.ToLower(s.Instance)
	d := Device{Addr: net.JoinHostPort(host, strconv.Itoa(s.Port)), ID: inst, Via: "mdns"}
	if g, err := strconv.Atoi(s.TXT["gen"]); err == nil && g >= 2 {
		d.Gen, d.App, d.Ver = g, s.TXT["app"], s.TXT["ver"]
		return d, true
	}
	if !strings.HasPrefix(inst, "shelly") {
		return Device{}, false
	}
	return d, true
}

// FromCoIoT turns a CoIoT message from a Gen1 device into a candidate at
// the sender's address, port 80.
func FromCoIoT(m shelly.CoIoTMessage, from *net.UDPAddr) (Device, bool) {
	if m.DevID == "" || from == nil {
		return Device{}, false
	}
	return Device{Addr: net.JoinHostPort(from.IP.String(), "80"), ID: strings.ToLower(m.DevID), Gen: 1, Model: m.DevType, Via: "coiot"}, true
}

// Probe reads GET /shelly at addr (host or host:port) and fills d from
// it. It fails when the answer is not a Shelly device's.
func Probe(ctx context.Context, hc *http.Client, addr string) (Device, error) {
	base, err := shelly.BaseURL(addr)
	if err != nil {
		return Device{}, err
	}
	if hc == nil {
		hc = &http.Client{Timeout: 3 * time.Second}
	}
	info, err := (&shelly.Client{Base: base, HTTP: hc}).Shelly(ctx)
	if err != nil {
		return Device{}, err
	}
	if info.MAC == "" || (info.Gen < 2 && info.Type == "") {
		return Device{}, errors.New("not a Shelly device")
	}
	return Device{Addr: strings.TrimPrefix(strings.TrimPrefix(base, "http://"), "https://"), ID: info.DefaultID(), Gen: info.Generation(),
		Model: info.ModelName(), App: info.App, Ver: info.Firmware(), Info: &info}, nil
}

// Options configure MDNS and Scan.
type Options struct {
	// HTTP is used for GET /shelly; a 3 s timeout client when nil.
	HTTP *http.Client
	// NoProbe returns DNS-SD candidates without reading GET /shelly.
	NoProbe bool
	// Workers bounds concurrent probes (16 when 0).
	Workers int
	// Port is the port Scan probes (80 when 0).
	Port int
}

func (o Options) workers() int {
	if o.Workers > 0 {
		return o.Workers
	}
	return 16
}

// MDNS browses _shelly._tcp and _http._tcp with b and returns the Shelly
// devices, one per address, confirmed with GET /shelly unless
// o.NoProbe. Devices that answer neither are left out.
func MDNS(ctx context.Context, b Browser, o Options) ([]Device, error) {
	byAddr := map[string]Device{}
	var errs []error
	for _, svc := range []string{ServiceShelly, ServiceHTTP} {
		list, err := b(ctx, svc)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, s := range list {
			d, ok := FromService(s)
			if !ok {
				continue
			}
			if old, seen := byAddr[d.Addr]; seen && old.Gen >= d.Gen {
				continue
			}
			byAddr[d.Addr] = d
		}
	}
	if len(byAddr) == 0 && len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	cands := make([]Device, 0, len(byAddr))
	for _, d := range byAddr {
		cands = append(cands, d)
	}
	if o.NoProbe {
		sortDevices(cands)
		return cands, nil
	}
	out := probeAll(ctx, o, cands, func(d Device) string { return d.Addr })
	for i := range out {
		out[i].Via = "mdns"
	}
	return out, nil
}

// Scan probes every address of prefix (IPv4 only, at most a /16) on
// o.Port and returns the Shelly devices that answered, sorted by address.
func Scan(ctx context.Context, prefix netip.Prefix, o Options) ([]Device, error) {
	prefix = prefix.Masked()
	if !prefix.Addr().Is4() || prefix.Bits() < 16 {
		return nil, errors.New("scan takes an IPv4 prefix of /16 or longer")
	}
	port := o.Port
	if port == 0 {
		port = 80
	}
	var cands []Device
	for a := prefix.Addr(); prefix.Contains(a); a = a.Next() {
		cands = append(cands, Device{Addr: net.JoinHostPort(a.String(), strconv.Itoa(port))})
	}
	out := probeAll(ctx, o, cands, func(d Device) string { return d.Addr })
	for i := range out {
		out[i].Via = "scan"
	}
	return out, ctx.Err()
}

func probeAll(ctx context.Context, o Options, cands []Device, addr func(Device) string) []Device {
	var (
		mu  sync.Mutex
		out []Device
		wg  sync.WaitGroup
	)
	sem := make(chan struct{}, o.workers())
	for _, c := range cands {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(c Device) {
			defer func() { <-sem; wg.Done() }()
			d, err := Probe(ctx, o.HTTP, addr(c))
			if err != nil {
				return
			}
			mu.Lock()
			out = append(out, d)
			mu.Unlock()
		}(c)
	}
	wg.Wait()
	sortDevices(out)
	return out
}

func sortDevices(ds []Device) {
	sort.Slice(ds, func(i, j int) bool {
		a, ea := netip.ParseAddrPort(ds[i].Addr)
		b, eb := netip.ParseAddrPort(ds[j].Addr)
		if ea == nil && eb == nil {
			return a.Compare(b) < 0
		}
		return ds[i].Addr < ds[j].Addr
	})
}

// ListenCoIoT reads CoIoT messages from l until ctx ends and calls found
// once per device (by id), confirmed with GET /shelly unless o.NoProbe.
func ListenCoIoT(ctx context.Context, l *shelly.CoIoTListener, o Options, found func(Device)) error {
	var mu sync.Mutex
	seen := map[string]bool{}
	return l.Serve(ctx, func(m shelly.CoIoTMessage, from *net.UDPAddr) {
		d, ok := FromCoIoT(m, from)
		if !ok {
			return
		}
		mu.Lock()
		dup := seen[d.ID]
		seen[d.ID] = true
		mu.Unlock()
		if dup {
			return
		}
		if !o.NoProbe {
			p, err := Probe(ctx, o.HTTP, d.Addr)
			if err != nil {
				mu.Lock()
				delete(seen, d.ID)
				mu.Unlock()
				return
			}
			p.Via = "coiot"
			d = p
		}
		found(d)
	})
}
