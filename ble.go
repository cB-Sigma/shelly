// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package shelly

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Shelly BLE RPC (Gen2+): the GATT service and its characteristics. A
// request is sent by writing its length (4 octets, big-endian) to TX
// control and the JSON to data; the answer's length is read from RX
// control and the answer from data until that many octets came.
const (
	BLEService   = "5f6d4f53-5f52-5043-5f53-56435f49445f"
	BLEData      = "5f6d4f53-5f52-5043-5f64-6174615f5f5f"
	BLETxControl = "5f6d4f53-5f52-5043-5f74-785f63746c5f"
	BLERxControl = "5f6d4f53-5f52-5043-5f72-785f63746c5f"
	// BLENamePrefix starts the advertised name of Shelly devices
	// ("ShellyPlus1PM-…"); compare without case.
	BLENamePrefix = "Shelly"
	// bleChunk is the most octets written or read at once (the smallest
	// ATT payload).
	bleChunk = 20
)

// GATT is a connected Bluetooth LE peer: characteristic writes (with
// response) and reads, by UUID. The application brings the Bluetooth
// stack; this package only frames the RPC.
type GATT interface {
	Write(ctx context.Context, char string, b []byte) error
	Read(ctx context.Context, char string) ([]byte, error)
}

// BLEDialer connects to a device over an adapter: target is its BLE
// address or advertised name, or "" for the first Shelly device found. It
// returns the peer, its advertised name and a close function.
type BLEDialer func(ctx context.Context, adapter, target string) (g GATT, name string, closeFn func(), err error)

// BLERPC sends one JSON-RPC request (with src as its "src") over Shelly's
// BLE RPC framing and returns its result.
func BLERPC(ctx context.Context, g GATT, src string, id int, method string, params map[string]any) (json.RawMessage, error) {
	req := map[string]any{"id": id, "src": src, "method": method}
	if params != nil {
		req["params"] = params
	}
	b, _ := json.Marshal(req)
	n := make([]byte, 4)
	binary.BigEndian.PutUint32(n, uint32(len(b)))
	if err := g.Write(ctx, BLETxControl, n); err != nil {
		return nil, fmt.Errorf("BLE: writing the request length: %w", err)
	}
	for off := 0; off < len(b); off += bleChunk {
		end := min(off+bleChunk, len(b))
		if err := g.Write(ctx, BLEData, b[off:end]); err != nil {
			return nil, fmt.Errorf("BLE: writing the request: %w", err)
		}
	}
	var want uint32
	for {
		rl, err := g.Read(ctx, BLERxControl)
		if err != nil {
			return nil, fmt.Errorf("BLE: reading the answer length: %w", err)
		}
		if len(rl) != 4 {
			return nil, fmt.Errorf("BLE: the answer length has %d octets, not 4", len(rl))
		}
		if want = binary.BigEndian.Uint32(rl); want > 0 {
			break
		}
		// Not ready yet.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	if want > 1<<20 {
		return nil, fmt.Errorf("BLE: an answer of %d octets is too long", want)
	}
	resp := make([]byte, 0, want)
	for uint32(len(resp)) < want {
		chunk, err := g.Read(ctx, BLEData)
		if err != nil {
			return nil, fmt.Errorf("BLE: reading the answer: %w", err)
		}
		if len(chunk) == 0 {
			return nil, errors.New("BLE: the device sent an empty chunk before the answer was complete")
		}
		resp = append(resp, chunk...)
	}
	var out struct {
		Result json.RawMessage `json:"result"`
		Error  *RPCError       `json:"error"`
	}
	if err := json.Unmarshal(resp[:want], &out); err != nil {
		return nil, fmt.Errorf("BLE: the answer is not JSON: %w", err)
	}
	if out.Error != nil {
		return nil, out.Error
	}
	return out.Result, nil
}
