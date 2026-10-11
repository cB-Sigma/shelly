// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package shelly

import (
	"context"
	"net/url"
	"strings"
	"time"
)

// Update tells the device to fetch and install the firmware image at
// imageURL: Shelly.Update with a url for gen 2 and later, GET /ota?url=
// for Gen1. The device restarts when it is done; WaitVersion follows it.
func (c *Client) Update(ctx context.Context, gen int, imageURL string) error {
	var err error
	if gen >= 2 {
		_, err = c.RPC(ctx, "Shelly.Update", map[string]any{"url": imageURL})
	} else {
		_, err = c.Get1(ctx, "/ota?url="+url.QueryEscape(imageURL))
	}
	return err
}

// WaitVersion reads GET /shelly every interval until the device reports a
// firmware version other than before (and containing want, when want is
// set), and returns what it read. Failed reads are skipped: the device is
// restarting. It returns ctx's error when ctx ends first.
func (c *Client) WaitVersion(ctx context.Context, before, want string, every time.Duration) (Info, error) {
	for {
		select {
		case <-ctx.Done():
			return Info{}, ctx.Err()
		case <-time.After(every):
		}
		info, err := c.Shelly(ctx)
		if err != nil {
			continue // restarting
		}
		now := info.Firmware()
		if now == before || (want != "" && !strings.Contains(now, want)) {
			continue
		}
		return info, nil
	}
}
