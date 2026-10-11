// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package shelly

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
)

// Gen1 settings beyond Wi-Fi. Gen1 devices change a setting with a GET of
// its endpoint and the new values as query parameters, and answer with
// the endpoint's settings after the change: /settings for the device,
// /settings/relay/n, /settings/light/n, /settings/roller/n,
// /settings/color/n, /settings/white/n for an output, /settings/actions
// for the URLs a device calls on its own events.

// Gen1Settings is the path with query that changes settings: section ""
// for /settings, else /settings/<section>/<n> (n < 0 leaves the index
// out, for sections such as cloud, mqtt, coiot or login).
func Gen1Settings(section string, n int, values url.Values) string {
	p := "/settings"
	if section != "" {
		p += "/" + section
		if n >= 0 {
			p += "/" + strconv.Itoa(n)
		}
	}
	if len(values) > 0 {
		p += "?" + values.Encode()
	}
	return p
}

// Settings1 reads or changes Gen1 settings (see Gen1Settings) and returns
// the endpoint's answer.
func (c *Client) Settings1(ctx context.Context, section string, n int, values url.Values) (map[string]any, error) {
	return c.Get1(ctx, Gen1Settings(section, n, values))
}

// Gen1ActionURL is one URL of a Gen1 action. Int is the time window of
// firmware that has one ("0000-0000": always).
type Gen1ActionURL struct {
	URL string `json:"url"`
	Int string `json:"int,omitempty"`
}

// Gen1Action is one slot of a Gen1 action (an event such as out_on_url
// or btn1_shortpush_url).
type Gen1Action struct {
	Index   int             `json:"index"`
	Enabled bool            `json:"enabled"`
	URLs    []Gen1ActionURL `json:"urls"`
}

// UnmarshalJSON reads URLs given as strings (older firmware) or as
// objects with url and int (firmware 1.10 and later).
func (a *Gen1Action) UnmarshalJSON(b []byte) error {
	var raw struct {
		Index   int               `json:"index"`
		Enabled bool              `json:"enabled"`
		URLs    []json.RawMessage `json:"urls"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	a.Index, a.Enabled, a.URLs = raw.Index, raw.Enabled, nil
	for _, u := range raw.URLs {
		var s string
		if json.Unmarshal(u, &s) == nil {
			a.URLs = append(a.URLs, Gen1ActionURL{URL: s})
			continue
		}
		var o Gen1ActionURL
		if err := json.Unmarshal(u, &o); err != nil {
			return fmt.Errorf("action url: %w", err)
		}
		a.URLs = append(a.URLs, o)
	}
	return nil
}

// Gen1Actions reads GET /settings/actions: the slots of each action by
// its name.
func (c *Client) Gen1Actions(ctx context.Context) (map[string][]Gen1Action, error) {
	b, err := c.Get(ctx, "/settings/actions")
	if err != nil {
		return nil, err
	}
	var r struct {
		Actions map[string][]Gen1Action `json:"actions"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("/settings/actions: %w", err)
	}
	return r.Actions, nil
}

// Gen1SetAction is the path with query that sets slot index of action
// name: enabled, and the URLs to call (an empty list clears them).
// Firmware that takes time windows reads urls[] as plain URLs too.
func Gen1SetAction(name string, index int, enabled bool, urls ...string) string {
	q := url.Values{}
	q.Set("index", strconv.Itoa(index))
	q.Set("name", name)
	q.Set("enabled", strconv.FormatBool(enabled))
	if len(urls) == 0 {
		q["urls[]"] = []string{""}
	} else {
		q["urls[]"] = urls
	}
	return "/settings/actions?" + q.Encode()
}

// Gen1ActionNames lists the action names of a /settings/actions answer,
// sorted.
func Gen1ActionNames(actions map[string][]Gen1Action) []string {
	out := make([]string, 0, len(actions))
	for k := range actions {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
