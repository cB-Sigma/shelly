// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package shelly

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"unicode/utf8"
)

// Caller sends one Gen2+ JSON-RPC request and returns its result. A
// *Client is one (HTTP); MQTTDevice and BLEDevice put the other
// transports behind it, and a WebSocket channel the application keeps
// can be wrapped the same way. The typed helpers below take a Caller, so
// they work over every transport.
type Caller interface {
	RPC(ctx context.Context, method string, params map[string]any) (json.RawMessage, error)
}

// CallerFunc adapts a function to Caller.
type CallerFunc func(ctx context.Context, method string, params map[string]any) (json.RawMessage, error)

// RPC calls f.
func (f CallerFunc) RPC(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	return f(ctx, method, params)
}

// MQTTDevice is one Gen2+ device over MQTT: Root is its topic prefix
// (the device id unless its MQTT topic_prefix was changed).
type MQTTDevice struct {
	M    *MQTT
	Root string
}

// RPC calls method on the device over the broker.
func (d MQTTDevice) RPC(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	return d.M.RPC(ctx, d.Root, method, params)
}

// BLEDevice is one Gen2+ device over BLE RPC through the application's
// GATT peer. Src is the "src" of requests; ids count up from 1.
type BLEDevice struct {
	G   GATT
	Src string

	mu sync.Mutex
	id int
}

// RPC calls method on the device over Bluetooth, one request at a time.
func (d *BLEDevice) RPC(ctx context.Context, method string, params map[string]any) (json.RawMessage, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.id++
	return BLERPC(ctx, d.G, d.Src, d.id, method, params)
}

// call runs method and decodes its result into out (when out is not nil).
func call(ctx context.Context, c Caller, method string, params map[string]any, out any) error {
	raw, err := c.RPC(ctx, method, params)
	if err != nil {
		return err
	}
	if out == nil || len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	return nil
}

// IsRPCError reports whether err carries a device error with code.
func IsRPCError(err error, code int) bool {
	var e *RPCError
	return errors.As(err, &e) && e.Code == code
}

// ---- status and configuration ----------------------------------------------

// GetStatus reads the whole status (Shelly.GetStatus).
func GetStatus(ctx context.Context, c Caller) (Status, error) {
	var s Status
	return s, call(ctx, c, "Shelly.GetStatus", nil, &s)
}

// GetConfig reads the whole configuration (Shelly.GetConfig).
func GetConfig(ctx context.Context, c Caller) (Config, error) {
	var cfg Config
	return cfg, call(ctx, c, "Shelly.GetConfig", nil, &cfg)
}

// GetDeviceInfo reads Shelly.GetDeviceInfo (the same fields as GET
// /shelly).
func GetDeviceInfo(ctx context.Context, c Caller) (Info, error) {
	var i Info
	return i, call(ctx, c, "Shelly.GetDeviceInfo", nil, &i)
}

// Component is one entry of Shelly.GetComponents: its key, and its
// status and configuration as the device sent them.
type Component struct {
	Key    string          `json:"key"`
	Status json.RawMessage `json:"status,omitempty"`
	Config json.RawMessage `json:"config,omitempty"`
	Attrs  json.RawMessage `json:"attrs,omitempty"`
}

// GetComponents lists the device's components (Shelly.GetComponents),
// reading every page; dynamicOnly lists only dynamic (virtual)
// components. It returns the configuration revision with them.
func GetComponents(ctx context.Context, c Caller, dynamicOnly bool) ([]Component, int, error) {
	var all []Component
	rev := 0
	for offset := 0; ; {
		var page struct {
			Components []Component `json:"components"`
			CfgRev     int         `json:"cfg_rev"`
			Offset     int         `json:"offset"`
			Total      int         `json:"total"`
		}
		p := map[string]any{"offset": offset}
		if dynamicOnly {
			p["dynamic_only"] = true
		}
		if err := call(ctx, c, "Shelly.GetComponents", p, &page); err != nil {
			return nil, 0, err
		}
		all = append(all, page.Components...)
		rev = page.CfgRev
		offset += len(page.Components)
		if len(page.Components) == 0 || offset >= page.Total {
			return all, rev, nil
		}
	}
}

// ---- outputs ------------------------------------------------------------------

// SetSwitch turns switch id on or off (Switch.Set); with toggleAfter > 0
// the device flips it back after that many seconds. It returns whether
// the switch was on.
func SetSwitch(ctx context.Context, c Caller, id int, on bool, toggleAfter float64) (wasOn bool, err error) {
	p := map[string]any{"id": id, "on": on}
	if toggleAfter > 0 {
		p["toggle_after"] = toggleAfter
	}
	var r struct {
		WasOn bool `json:"was_on"`
	}
	err = call(ctx, c, "Switch.Set", p, &r)
	return r.WasOn, err
}

// ToggleSwitch flips switch id (Switch.Toggle).
func ToggleSwitch(ctx context.Context, c Caller, id int) (wasOn bool, err error) {
	var r struct {
		WasOn bool `json:"was_on"`
	}
	err = call(ctx, c, "Switch.Toggle", map[string]any{"id": id}, &r)
	return r.WasOn, err
}

// GetSwitch reads switch id's status (Switch.GetStatus).
func GetSwitch(ctx context.Context, c Caller, id int) (SwitchStatus, error) {
	var s SwitchStatus
	return s, call(ctx, c, "Switch.GetStatus", map[string]any{"id": id}, &s)
}

// GetCover reads cover id's status (Cover.GetStatus).
func GetCover(ctx context.Context, c Caller, id int) (CoverStatus, error) {
	var s CoverStatus
	return s, call(ctx, c, "Cover.GetStatus", map[string]any{"id": id}, &s)
}

// MoveCover runs Cover.Open, Cover.Close or Cover.Stop (move is open,
// close or stop); with duration > 0 an open or close runs for that many
// seconds.
func MoveCover(ctx context.Context, c Caller, id int, move string, duration float64) error {
	var m string
	switch move {
	case "open":
		m = "Cover.Open"
	case "close":
		m = "Cover.Close"
	case "stop":
		m = "Cover.Stop"
	default:
		return fmt.Errorf("cover move %q is not open, close or stop", move)
	}
	p := map[string]any{"id": id}
	if duration > 0 && move != "stop" {
		p["duration"] = duration
	}
	return call(ctx, c, m, p, nil)
}

// CoverToPosition moves cover id to pos percent open (Cover.GoToPosition).
func CoverToPosition(ctx context.Context, c Caller, id int, pos float64) error {
	return call(ctx, c, "Cover.GoToPosition", map[string]any{"id": id, "pos": pos}, nil)
}

// LightSet is what LightSetParams sends; nil fields are left as they are.
type LightSet struct {
	On         *bool
	Brightness *float64 // percent
	RGB        []int    // red, green, blue 0-255 (rgb, rgbw)
	White      *float64 // 0-255 (rgbw)
	CT         *float64 // kelvin (cct)
	Transition *float64 // seconds
	ToggleAft  *float64 // seconds
}

// SetLight sets a light-like component (typ light, rgb, rgbw or cct)
// with <Type>.Set.
func SetLight(ctx context.Context, c Caller, typ string, id int, s LightSet) error {
	p := map[string]any{"id": id}
	if s.On != nil {
		p["on"] = *s.On
	}
	if s.Brightness != nil {
		p["brightness"] = *s.Brightness
	}
	if s.RGB != nil {
		if len(s.RGB) != 3 {
			return errors.New("rgb takes three values")
		}
		p["rgb"] = s.RGB
	}
	if s.White != nil {
		p["white"] = *s.White
	}
	if s.CT != nil {
		p["ct"] = *s.CT
	}
	if s.Transition != nil {
		p["transition_duration"] = *s.Transition
	}
	if s.ToggleAft != nil {
		p["toggle_after"] = *s.ToggleAft
	}
	return call(ctx, c, MethodPrefix(typ)+".Set", p, nil)
}

// SetConfig sets part of a component's configuration with
// <Type>.SetConfig (key "switch:0", or "sys", "wifi" without an id) and
// returns whether the device must restart for it.
func SetConfig(ctx context.Context, c Caller, key string, config map[string]any) (restartRequired bool, err error) {
	typ, id, hasID := SplitKey(key)
	p := map[string]any{"config": config}
	if hasID {
		p["id"] = id
	}
	var r struct {
		Restart bool `json:"restart_required"`
	}
	err = call(ctx, c, MethodPrefix(typ)+".SetConfig", p, &r)
	return r.Restart, err
}

// ---- scripts --------------------------------------------------------------------

// Script is an entry of Script.List.
type Script struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Enable  bool   `json:"enable"`
	Running bool   `json:"running"`
}

// ScriptStatus is Script.GetStatus.
type ScriptStatus struct {
	ID      int      `json:"id"`
	Running bool     `json:"running"`
	MemUsed int      `json:"mem_used,omitempty"`
	MemPeak int      `json:"mem_peak,omitempty"`
	MemFree int      `json:"mem_free,omitempty"`
	Errors  []string `json:"errors,omitempty"`
}

// DefaultScriptChunk is how many octets of code ScriptPutCode sends per
// request when no size is given.
const DefaultScriptChunk = 1024

// ListScripts lists the device's scripts (Script.List).
func ListScripts(ctx context.Context, c Caller) ([]Script, error) {
	var r struct {
		Scripts []Script `json:"scripts"`
	}
	err := call(ctx, c, "Script.List", nil, &r)
	return r.Scripts, err
}

// CreateScript makes an empty script named name (Script.Create).
func CreateScript(ctx context.Context, c Caller, name string) (int, error) {
	var r struct {
		ID int `json:"id"`
	}
	err := call(ctx, c, "Script.Create", map[string]any{"name": name}, &r)
	return r.ID, err
}

// PutScriptCode replaces script id's code with code, sent in chunks of
// at most chunk octets (DefaultScriptChunk when 0), split between UTF-8
// characters: the first replaces, the rest append (Script.PutCode).
func PutScriptCode(ctx context.Context, c Caller, id int, code string, chunk int) error {
	if chunk <= 0 {
		chunk = DefaultScriptChunk
	}
	if chunk < utf8.UTFMax {
		chunk = utf8.UTFMax
	}
	first := true
	for first || code != "" {
		n := min(chunk, len(code))
		for n < len(code) && n > 0 && !utf8.RuneStart(code[n]) {
			n--
		}
		p := map[string]any{"id": id, "code": code[:n], "append": !first}
		if err := call(ctx, c, "Script.PutCode", p, nil); err != nil {
			return err
		}
		code, first = code[n:], false
	}
	return nil
}

// GetScriptCode reads script id's code whole, page by page
// (Script.GetCode with offset).
func GetScriptCode(ctx context.Context, c Caller, id int) (string, error) {
	var out []byte
	for {
		var r struct {
			Data string `json:"data"`
			Left int    `json:"left"`
		}
		if err := call(ctx, c, "Script.GetCode", map[string]any{"id": id, "offset": len(out)}, &r); err != nil {
			return "", err
		}
		out = append(out, r.Data...)
		if r.Left <= 0 || r.Data == "" {
			return string(out), nil
		}
	}
}

// StartScript runs script id (Script.Start); wasRunning says whether it
// already ran.
func StartScript(ctx context.Context, c Caller, id int) (wasRunning bool, err error) {
	var r struct {
		Was bool `json:"was_running"`
	}
	err = call(ctx, c, "Script.Start", map[string]any{"id": id}, &r)
	return r.Was, err
}

// StopScript stops script id (Script.Stop).
func StopScript(ctx context.Context, c Caller, id int) (wasRunning bool, err error) {
	var r struct {
		Was bool `json:"was_running"`
	}
	err = call(ctx, c, "Script.Stop", map[string]any{"id": id}, &r)
	return r.Was, err
}

// DeleteScript removes script id (Script.Delete).
func DeleteScript(ctx context.Context, c Caller, id int) error {
	return call(ctx, c, "Script.Delete", map[string]any{"id": id}, nil)
}

// EvalScript evaluates code in running script id's context (Script.Eval)
// and returns the result as the device printed it.
func EvalScript(ctx context.Context, c Caller, id int, code string) (string, error) {
	var r struct {
		Result string `json:"result"`
	}
	err := call(ctx, c, "Script.Eval", map[string]any{"id": id, "code": code}, &r)
	return r.Result, err
}

// GetScriptStatus reads Script.GetStatus.
func GetScriptStatus(ctx context.Context, c Caller, id int) (ScriptStatus, error) {
	var s ScriptStatus
	return s, call(ctx, c, "Script.GetStatus", map[string]any{"id": id}, &s)
}

// SetScriptConfig renames script id or sets whether it runs at boot
// (Script.SetConfig); nil fields are left as they are.
func SetScriptConfig(ctx context.Context, c Caller, id int, name *string, enable *bool) error {
	cfg := map[string]any{}
	if name != nil {
		cfg["name"] = *name
	}
	if enable != nil {
		cfg["enable"] = *enable
	}
	return call(ctx, c, "Script.SetConfig", map[string]any{"id": id, "config": cfg}, nil)
}

// ---- schedules ------------------------------------------------------------------

// ScheduleCall is one RPC call a schedule job makes.
type ScheduleCall struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params,omitempty"`
}

// ScheduleJob is a schedule job. Timespec has six space-separated
// fields (seconds, minutes, hours, day of month, month, day of week) in
// the cron style the device documents, or starts with @sunrise or
// @sunset with an optional offset. ID is set by the device.
type ScheduleJob struct {
	ID       int            `json:"id,omitempty"`
	Enable   bool           `json:"enable"`
	Timespec string         `json:"timespec"`
	Calls    []ScheduleCall `json:"calls"`
}

func (j ScheduleJob) params(withID bool) map[string]any {
	calls := make([]any, 0, len(j.Calls))
	for _, cl := range j.Calls {
		m := map[string]any{"method": cl.Method}
		if cl.Params != nil {
			m["params"] = cl.Params
		}
		calls = append(calls, m)
	}
	p := map[string]any{"enable": j.Enable, "timespec": j.Timespec, "calls": calls}
	if withID {
		p["id"] = j.ID
	}
	return p
}

type revResult struct {
	ID  int `json:"id"`
	Rev int `json:"rev"`
}

// CreateSchedule adds a job (Schedule.Create) and returns its id and the
// schedules' new revision.
func CreateSchedule(ctx context.Context, c Caller, j ScheduleJob) (id, rev int, err error) {
	var r revResult
	err = call(ctx, c, "Schedule.Create", j.params(false), &r)
	return r.ID, r.Rev, err
}

// UpdateSchedule replaces job j.ID (Schedule.Update).
func UpdateSchedule(ctx context.Context, c Caller, j ScheduleJob) (rev int, err error) {
	if j.ID == 0 {
		return 0, errors.New("the schedule job has no id")
	}
	var r revResult
	err = call(ctx, c, "Schedule.Update", j.params(true), &r)
	return r.Rev, err
}

// ListSchedules lists the jobs (Schedule.List).
func ListSchedules(ctx context.Context, c Caller) ([]ScheduleJob, int, error) {
	var r struct {
		Jobs []ScheduleJob `json:"jobs"`
		Rev  int           `json:"rev"`
	}
	err := call(ctx, c, "Schedule.List", nil, &r)
	return r.Jobs, r.Rev, err
}

// DeleteSchedule removes job id (Schedule.Delete).
func DeleteSchedule(ctx context.Context, c Caller, id int) (rev int, err error) {
	var r revResult
	err = call(ctx, c, "Schedule.Delete", map[string]any{"id": id}, &r)
	return r.Rev, err
}

// DeleteAllSchedules removes every job (Schedule.DeleteAll).
func DeleteAllSchedules(ctx context.Context, c Caller) (rev int, err error) {
	var r revResult
	err = call(ctx, c, "Schedule.DeleteAll", nil, &r)
	return r.Rev, err
}

// ---- webhooks -------------------------------------------------------------------

// Webhook is a webhook (an action): when Event happens on component
// instance CID, the device calls URLs. Condition, RepeatPeriod and
// ActiveBetween are optional.
type Webhook struct {
	ID            int      `json:"id,omitempty"`
	CID           int      `json:"cid"`
	Enable        bool     `json:"enable"`
	Event         string   `json:"event"`
	Name          string   `json:"name,omitempty"`
	URLs          []string `json:"urls"`
	Condition     string   `json:"condition,omitempty"`
	RepeatPeriod  int      `json:"repeat_period,omitempty"`
	ActiveBetween []string `json:"active_between,omitempty"`
	SSLCA         string   `json:"ssl_ca,omitempty"`
}

func (h Webhook) params(withID bool) map[string]any {
	urls := h.URLs
	if urls == nil {
		urls = []string{}
	}
	p := map[string]any{"cid": h.CID, "enable": h.Enable, "event": h.Event, "urls": urls}
	if h.Name != "" {
		p["name"] = h.Name
	}
	if h.Condition != "" {
		p["condition"] = h.Condition
	}
	if h.RepeatPeriod != 0 {
		p["repeat_period"] = h.RepeatPeriod
	}
	if h.ActiveBetween != nil {
		p["active_between"] = h.ActiveBetween
	}
	if h.SSLCA != "" {
		p["ssl_ca"] = h.SSLCA
	}
	if withID {
		p["id"] = h.ID
	}
	return p
}

// CreateWebhook adds a webhook (Webhook.Create).
func CreateWebhook(ctx context.Context, c Caller, h Webhook) (id, rev int, err error) {
	var r revResult
	err = call(ctx, c, "Webhook.Create", h.params(false), &r)
	return r.ID, r.Rev, err
}

// UpdateWebhook replaces webhook h.ID (Webhook.Update).
func UpdateWebhook(ctx context.Context, c Caller, h Webhook) (rev int, err error) {
	if h.ID == 0 {
		return 0, errors.New("the webhook has no id")
	}
	var r revResult
	err = call(ctx, c, "Webhook.Update", h.params(true), &r)
	return r.Rev, err
}

// ListWebhooks lists the webhooks (Webhook.List).
func ListWebhooks(ctx context.Context, c Caller) ([]Webhook, int, error) {
	var r struct {
		Hooks []Webhook `json:"hooks"`
		Rev   int       `json:"rev"`
	}
	err := call(ctx, c, "Webhook.List", nil, &r)
	return r.Hooks, r.Rev, err
}

// DeleteWebhook removes webhook id (Webhook.Delete).
func DeleteWebhook(ctx context.Context, c Caller, id int) (rev int, err error) {
	var r revResult
	err = call(ctx, c, "Webhook.Delete", map[string]any{"id": id}, &r)
	return r.Rev, err
}

// DeleteAllWebhooks removes every webhook (Webhook.DeleteAll).
func DeleteAllWebhooks(ctx context.Context, c Caller) (rev int, err error) {
	var r revResult
	err = call(ctx, c, "Webhook.DeleteAll", nil, &r)
	return r.Rev, err
}

// WebhookEvents lists the events webhooks can be made for
// (Webhook.ListSupported), sorted. Firmware before 1.0 answered a list
// under hook_types; later firmware answers an object under types.
func WebhookEvents(ctx context.Context, c Caller) ([]string, error) {
	var r struct {
		HookTypes []string                   `json:"hook_types"`
		Types     map[string]json.RawMessage `json:"types"`
	}
	if err := call(ctx, c, "Webhook.ListSupported", nil, &r); err != nil {
		return nil, err
	}
	out := append([]string(nil), r.HookTypes...)
	for k := range r.Types {
		out = append(out, k)
	}
	sortStrings(out)
	return out, nil
}

// ---- KVS ------------------------------------------------------------------------

// KVSItem is one entry of the key-value store.
type KVSItem struct {
	Key   string          `json:"key"`
	Etag  string          `json:"etag"`
	Value json.RawMessage `json:"value"`
}

// KVSSet stores value (any JSON value) under key (KVS.Set). With etag
// set, the device refuses the write unless the stored entry still has
// that etag.
func KVSSet(ctx context.Context, c Caller, key string, value any, etag string) (newEtag string, rev int, err error) {
	p := map[string]any{"key": key, "value": value}
	if etag != "" {
		p["etag"] = etag
	}
	var r struct {
		Etag string `json:"etag"`
		Rev  int    `json:"rev"`
	}
	err = call(ctx, c, "KVS.Set", p, &r)
	return r.Etag, r.Rev, err
}

// KVSGet reads key (KVS.Get).
func KVSGet(ctx context.Context, c Caller, key string) (KVSItem, error) {
	var it KVSItem
	err := call(ctx, c, "KVS.Get", map[string]any{"key": key}, &it)
	it.Key = key
	return it, err
}

// KVSGetMany reads the entries whose keys match (KVS.GetMany; "*" is a
// wildcard, "" all), every page. Firmware before 1.5 answers an object by
// key, later firmware a list with offset and total; both are read.
func KVSGetMany(ctx context.Context, c Caller, match string) ([]KVSItem, error) {
	var out []KVSItem
	for {
		p := map[string]any{"offset": len(out)}
		if match != "" {
			p["match"] = match
		}
		raw, err := c.RPC(ctx, "KVS.GetMany", p)
		if err != nil {
			return nil, err
		}
		var r struct {
			Items json.RawMessage `json:"items"`
			Total *int            `json:"total"`
		}
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, fmt.Errorf("KVS.GetMany: %w", err)
		}
		var list []KVSItem
		if err := json.Unmarshal(r.Items, &list); err != nil {
			var byKey map[string]KVSItem
			if err := json.Unmarshal(r.Items, &byKey); err != nil {
				return nil, fmt.Errorf("KVS.GetMany items: %w", err)
			}
			keys := make([]string, 0, len(byKey))
			for k := range byKey {
				keys = append(keys, k)
			}
			sortStrings(keys)
			for _, k := range keys {
				it := byKey[k]
				it.Key = k
				out = append(out, it)
			}
			return out, nil // the old form has no pages
		}
		out = append(out, list...)
		if r.Total == nil || len(list) == 0 || len(out) >= *r.Total {
			return out, nil
		}
	}
}

// KVSList lists the keys that match (KVS.List) with their etags.
func KVSList(ctx context.Context, c Caller, match string) (map[string]string, int, error) {
	p := map[string]any{}
	if match != "" {
		p["match"] = match
	}
	var r struct {
		Keys map[string]struct {
			Etag string `json:"etag"`
		} `json:"keys"`
		Rev int `json:"rev"`
	}
	if err := call(ctx, c, "KVS.List", p, &r); err != nil {
		return nil, 0, err
	}
	out := make(map[string]string, len(r.Keys))
	for k, v := range r.Keys {
		out[k] = v.Etag
	}
	return out, r.Rev, nil
}

// KVSDelete removes key (KVS.Delete); with etag set only if it still
// matches.
func KVSDelete(ctx context.Context, c Caller, key, etag string) (rev int, err error) {
	p := map[string]any{"key": key}
	if etag != "" {
		p["etag"] = etag
	}
	var r revResult
	err = call(ctx, c, "KVS.Delete", p, &r)
	return r.Rev, err
}

// ---- virtual components ------------------------------------------------------------

// AddVirtual creates a dynamic (virtual) component (Virtual.Add): typ is
// boolean, number, text, enum, group or button; id 0 lets the device pick
// one (from 200). It returns the new component's id.
func AddVirtual(ctx context.Context, c Caller, typ string, config map[string]any, id int) (int, error) {
	p := map[string]any{"type": typ}
	if config != nil {
		p["config"] = config
	}
	if id > 0 {
		p["id"] = id
	}
	var r struct {
		ID int `json:"id"`
	}
	err := call(ctx, c, "Virtual.Add", p, &r)
	return r.ID, err
}

// DeleteVirtual removes a dynamic component by key ("number:200").
func DeleteVirtual(ctx context.Context, c Caller, key string) error {
	return call(ctx, c, "Virtual.Delete", map[string]any{"key": key}, nil)
}

// SetVirtual sets a dynamic component's value (<Type>.Set with id and
// value), key as "boolean:200".
func SetVirtual(ctx context.Context, c Caller, key string, value any) error {
	typ, id, ok := SplitKey(key)
	if !ok {
		return fmt.Errorf("%q is not a component key", key)
	}
	return call(ctx, c, MethodPrefix(typ)+".Set", map[string]any{"id": id, "value": value}, nil)
}
