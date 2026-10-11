// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package shelly

import (
	"math"
	"net/url"
	"strconv"
	"strings"
)

// Gen1 requests: each builder returns the path with query of a Gen1 GET
// (for Client.Get1, or MQTT.Command1 through Gen1MQTT). Levels and
// positions are rounded to whole numbers; a transition is in seconds and
// sent in milliseconds.

func gen1Path(kind string, n int, q url.Values) string {
	return "/" + kind + "/" + strconv.Itoa(n) + "?" + q.Encode()
}

// Gen1Turn switches output n of kind relay or light: turn is on, off or
// toggle.
func Gen1Turn(kind string, n int, turn string) string {
	return gen1Path(kind, n, url.Values{"turn": {turn}})
}

// Gen1Level dims light output n to level percent (0 turns it off). A
// light in colour mode (a Bulb or RGBW2 with mode color in its status)
// dims with gain on /color/n, since /light/n's brightness applies to white
// mode only; colourMode selects that form.
func Gen1Level(n int, level float64, colourMode bool, transition *float64) string {
	q := url.Values{}
	if level == 0 {
		q.Set("turn", "off")
	} else {
		q.Set("turn", "on")
		if colourMode {
			q.Set("gain", strconv.Itoa(int(math.Round(level))))
		} else {
			q.Set("brightness", strconv.Itoa(int(math.Round(level))))
		}
	}
	if transition != nil {
		q.Set("transition", strconv.Itoa(int(*transition*1000)))
	}
	if colourMode {
		return gen1Path("color", n, q)
	}
	return gen1Path("light", n, q)
}

// Gen1Colour turns colour output n on with red, green and blue, and gain
// (percent) when set.
func Gen1Colour(n int, r, g, b uint8, gain *float64) string {
	q := url.Values{}
	q.Set("turn", "on")
	q.Set("red", strconv.Itoa(int(r)))
	q.Set("green", strconv.Itoa(int(g)))
	q.Set("blue", strconv.Itoa(int(b)))
	if gain != nil {
		q.Set("gain", strconv.Itoa(int(math.Round(*gain))))
	}
	return gen1Path("color", n, q)
}

// Gen1Roller moves roller n: open, close or stop.
func Gen1Roller(n int, move string) string {
	return gen1Path("roller", n, url.Values{"go": {move}})
}

// Gen1RollerPos moves roller n to pos percent (100 open).
func Gen1RollerPos(n int, pos float64) string {
	return gen1Path("roller", n, url.Values{"go": {"to_pos"}, "roller_pos": {strconv.Itoa(int(math.Round(pos)))}})
}

// SplitKey splits a Gen2+ component key ("switch:1") into its type and
// id; ok is false for a key without a numeric id ("sys").
func SplitKey(k string) (typ string, id int, ok bool) {
	t, idStr, found := strings.Cut(k, ":")
	if !found {
		return k, 0, false
	}
	n, err := strconv.Atoi(idStr)
	if err != nil {
		return t, 0, false
	}
	return t, n, true
}

// MethodPrefix is the RPC namespace of a Gen2+ component type: switch →
// Switch, light → Light, rgb → RGB, rgbw → RGBW, cct → CCT, cover →
// Cover.
func MethodPrefix(typ string) string {
	switch typ {
	case "rgb":
		return "RGB"
	case "rgbw":
		return "RGBW"
	case "cct":
		return "CCT"
	}
	if typ == "" {
		return ""
	}
	return strings.ToUpper(typ[:1]) + typ[1:]
}
