// Copyright 2026 Abhinav Gautam
// SPDX-License-Identifier: Apache-2.0

package shelly

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
)

// Typed views of Gen2+ status and configuration. The maps stay the
// source: Status and Config keep every component as the device sent it,
// so a component or field this package does not know still reaches the
// application, and the typed accessors read the common components out of
// it. Optional readings are pointers: nil means the device did not send
// the field (a switch without a meter has no apower), not zero.

// Status is a Gen2+ device's status (Shelly.GetStatus, NotifyFullStatus)
// by component key ("switch:0", "sys").
type Status map[string]json.RawMessage

// Config is a Gen2+ device's configuration (Shelly.GetConfig) by
// component key.
type Config map[string]json.RawMessage

// ParseStatus reads a Shelly.GetStatus result.
func ParseStatus(b []byte) (Status, error) {
	var s Status
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("status: %w", err)
	}
	return s, nil
}

// ParseConfig reads a Shelly.GetConfig result.
func ParseConfig(b []byte) (Config, error) {
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return c, nil
}

// componentKeys lists the keys of one component type, by id.
func componentKeys(m map[string]json.RawMessage, typ string) []string {
	var keys []string
	for k := range m {
		if t, _, ok := SplitKey(k); ok && t == typ {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		_, a, _ := SplitKey(keys[i])
		_, b, _ := SplitKey(keys[j])
		return a < b
	})
	return keys
}

func decodeKey(m map[string]json.RawMessage, key string, v any) bool {
	raw, ok := m[key]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return false
	}
	return json.Unmarshal(raw, v) == nil
}

func idKey(typ string, id int) string { return typ + ":" + strconv.Itoa(id) }

// Keys lists the keys of one component type ("switch") by id.
func (s Status) Keys(typ string) []string { return componentKeys(s, typ) }

// Decode reads one component's status into v; false when the device has
// no such component or it does not fit v.
func (s Status) Decode(key string, v any) bool { return decodeKey(s, key, v) }

// Keys lists the keys of one component type by id.
func (c Config) Keys(typ string) []string { return componentKeys(c, typ) }

// Decode reads one component's configuration into v.
func (c Config) Decode(key string, v any) bool { return decodeKey(c, key, v) }

// Energy is an energy counter: Total in watt-hours, and the last three
// whole minutes in milliwatt-hours with the Unix time of the minute
// they end at.
type Energy struct {
	Total    float64   `json:"total"`
	ByMinute []float64 `json:"by_minute,omitempty"`
	MinuteTS int64     `json:"minute_ts,omitempty"`
}

// Temperature is a temperature in Celsius and Fahrenheit (nil when the
// sensor has no reading).
type Temperature struct {
	C *float64 `json:"tC"`
	F *float64 `json:"tF"`
}

// Meter holds the readings a metering output reports.
type Meter struct {
	APower     *float64     `json:"apower,omitempty"`
	Voltage    *float64     `json:"voltage,omitempty"`
	Current    *float64     `json:"current,omitempty"`
	PF         *float64     `json:"pf,omitempty"`
	Freq       *float64     `json:"freq,omitempty"`
	AEnergy    *Energy      `json:"aenergy,omitempty"`
	RetAEnergy *Energy      `json:"ret_aenergy,omitempty"`
	Temp       *Temperature `json:"temperature,omitempty"`
}

// SwitchStatus is a switch component's status.
type SwitchStatus struct {
	ID             int      `json:"id"`
	Source         string   `json:"source,omitempty"`
	Output         bool     `json:"output"`
	TimerStartedAt *float64 `json:"timer_started_at,omitempty"`
	TimerDuration  *float64 `json:"timer_duration,omitempty"`
	Errors         []string `json:"errors,omitempty"`
	Meter
}

// CoverStatus is a cover component's status. State is open, closed,
// opening, closing, stopped or calibrating; positions are percent open
// and nil when the cover is not calibrated.
type CoverStatus struct {
	ID            int      `json:"id"`
	Source        string   `json:"source,omitempty"`
	State         string   `json:"state"`
	CurrentPos    *float64 `json:"current_pos,omitempty"`
	TargetPos     *float64 `json:"target_pos,omitempty"`
	SlatPos       *float64 `json:"slat_pos,omitempty"`
	MoveTimeout   *float64 `json:"move_timeout,omitempty"`
	MoveStartedAt *float64 `json:"move_started_at,omitempty"`
	PosControl    bool     `json:"pos_control,omitempty"`
	LastDirection string   `json:"last_direction,omitempty"`
	Errors        []string `json:"errors,omitempty"`
	Meter
}

// LightStatus is the status of a light, rgb, rgbw or cct component.
// Brightness is percent; RGB is red, green and blue 0-255; White is the
// white channel 0-255 (rgbw); CT is the colour temperature in kelvin
// (cct).
type LightStatus struct {
	ID             int      `json:"id"`
	Source         string   `json:"source,omitempty"`
	Output         bool     `json:"output"`
	Brightness     *float64 `json:"brightness,omitempty"`
	RGB            []int    `json:"rgb,omitempty"`
	White          *float64 `json:"white,omitempty"`
	CT             *float64 `json:"ct,omitempty"`
	TimerStartedAt *float64 `json:"timer_started_at,omitempty"`
	TimerDuration  *float64 `json:"timer_duration,omitempty"`
	Errors         []string `json:"errors,omitempty"`
	Meter
}

// InputStatus is an input component's status: State for a switch or
// button input (nil while unknown), Percent for an analog input, Counts
// and Freq for a counter input.
type InputStatus struct {
	ID      int          `json:"id"`
	State   *bool        `json:"state,omitempty"`
	Percent *float64     `json:"percent,omitempty"`
	Counts  *InputCounts `json:"counts,omitempty"`
	Freq    *float64     `json:"freq,omitempty"`
	Errors  []string     `json:"errors,omitempty"`
}

// InputCounts is a counter input's pulse counts.
type InputCounts struct {
	Total    float64   `json:"total"`
	XTotal   *float64  `json:"xtotal,omitempty"`
	ByMinute []float64 `json:"by_minute,omitempty"`
	MinuteTS int64     `json:"minute_ts,omitempty"`
}

// EMPhase is one phase of a three-phase energy meter.
type EMPhase struct {
	Current   *float64 `json:"current,omitempty"`
	Voltage   *float64 `json:"voltage,omitempty"`
	ActPower  *float64 `json:"act_power,omitempty"`
	AprtPower *float64 `json:"aprt_power,omitempty"`
	PF        *float64 `json:"pf,omitempty"`
	Freq      *float64 `json:"freq,omitempty"`
}

// EMStatus is a three-phase energy meter's status (em). The phases are
// read from the a_, b_ and c_ prefixed fields.
type EMStatus struct {
	ID             int      `json:"id"`
	A, B, C        EMPhase  `json:"-"`
	NCurrent       *float64 `json:"n_current,omitempty"`
	TotalCurrent   *float64 `json:"total_current,omitempty"`
	TotalActPower  *float64 `json:"total_act_power,omitempty"`
	TotalAprtPower *float64 `json:"total_aprt_power,omitempty"`
	Errors         []string `json:"errors,omitempty"`
}

// UnmarshalJSON reads the phase-prefixed fields into A, B and C.
func (e *EMStatus) UnmarshalJSON(b []byte) error {
	type plain EMStatus
	if err := json.Unmarshal(b, (*plain)(e)); err != nil {
		return err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	for p, ph := range map[string]*EMPhase{"a_": &e.A, "b_": &e.B, "c_": &e.C} {
		sub := map[string]json.RawMessage{}
		for _, f := range []string{"current", "voltage", "act_power", "aprt_power", "pf", "freq"} {
			if v, ok := m[p+f]; ok {
				sub[f] = v
			}
		}
		bb, _ := json.Marshal(sub)
		if err := json.Unmarshal(bb, ph); err != nil {
			return err
		}
	}
	return nil
}

// EMDataStatus is a three-phase meter's energy totals (emdata), in
// watt-hours.
type EMDataStatus struct {
	ID                 int      `json:"id"`
	ATotalActEnergy    *float64 `json:"a_total_act_energy,omitempty"`
	ATotalActRetEnergy *float64 `json:"a_total_act_ret_energy,omitempty"`
	BTotalActEnergy    *float64 `json:"b_total_act_energy,omitempty"`
	BTotalActRetEnergy *float64 `json:"b_total_act_ret_energy,omitempty"`
	CTotalActEnergy    *float64 `json:"c_total_act_energy,omitempty"`
	CTotalActRetEnergy *float64 `json:"c_total_act_ret_energy,omitempty"`
	TotalAct           *float64 `json:"total_act,omitempty"`
	TotalActRet        *float64 `json:"total_act_ret,omitempty"`
}

// EM1Status is a single-phase energy meter channel's status (em1).
type EM1Status struct {
	ID int `json:"id"`
	EMPhase
	Errors []string `json:"errors,omitempty"`
}

// EM1DataStatus is a single-phase channel's energy totals (em1data).
type EM1DataStatus struct {
	ID                int      `json:"id"`
	TotalActEnergy    *float64 `json:"total_act_energy,omitempty"`
	TotalActRetEnergy *float64 `json:"total_act_ret_energy,omitempty"`
}

// PM1Status is a power meter's status (pm1).
type PM1Status struct {
	ID     int      `json:"id"`
	Errors []string `json:"errors,omitempty"`
	Meter
}

// TemperatureStatus is a temperature sensor's status.
type TemperatureStatus struct {
	ID     int      `json:"id"`
	C      *float64 `json:"tC"`
	F      *float64 `json:"tF"`
	Errors []string `json:"errors,omitempty"`
}

// HumidityStatus is a humidity sensor's status (relative humidity in
// percent).
type HumidityStatus struct {
	ID     int      `json:"id"`
	RH     *float64 `json:"rh"`
	Errors []string `json:"errors,omitempty"`
}

// DevicePowerStatus is a battery device's power status.
type DevicePowerStatus struct {
	ID      int `json:"id"`
	Battery struct {
		V       *float64 `json:"V"`
		Percent *float64 `json:"percent"`
	} `json:"battery"`
	External *struct {
		Present bool `json:"present"`
	} `json:"external,omitempty"`
	Errors []string `json:"errors,omitempty"`
}

// SysStatus is the sys component's status.
type SysStatus struct {
	MAC              string  `json:"mac"`
	RestartRequired  bool    `json:"restart_required"`
	Time             string  `json:"time,omitempty"`
	UnixTime         int64   `json:"unixtime,omitempty"`
	Uptime           float64 `json:"uptime"`
	RAMSize          int     `json:"ram_size,omitempty"`
	RAMFree          int     `json:"ram_free,omitempty"`
	FSSize           int     `json:"fs_size,omitempty"`
	FSFree           int     `json:"fs_free,omitempty"`
	CfgRev           int     `json:"cfg_rev,omitempty"`
	KVSRev           int     `json:"kvs_rev,omitempty"`
	ScheduleRev      int     `json:"schedule_rev,omitempty"`
	WebhookRev       int     `json:"webhook_rev,omitempty"`
	WakeupPeriod     int     `json:"wakeup_period,omitempty"`
	ResetReason      int     `json:"reset_reason,omitempty"`
	AvailableUpdates map[string]struct {
		Version string `json:"version"`
	} `json:"available_updates,omitempty"`
}

// WiFiStatus is the wifi component's status.
type WiFiStatus struct {
	StaIP         *string  `json:"sta_ip"`
	Status        string   `json:"status"`
	SSID          *string  `json:"ssid"`
	RSSI          *float64 `json:"rssi"`
	APClientCount *int     `json:"ap_client_count,omitempty"`
}

// Switch reads switch:id.
func (s Status) Switch(id int) (v SwitchStatus, ok bool) {
	return v, s.Decode(idKey("switch", id), &v)
}

// Cover reads cover:id.
func (s Status) Cover(id int) (v CoverStatus, ok bool) {
	return v, s.Decode(idKey("cover", id), &v)
}

// Light reads a light-like component: typ is light, rgb, rgbw or cct.
func (s Status) Light(typ string, id int) (v LightStatus, ok bool) {
	return v, s.Decode(idKey(typ, id), &v)
}

// Input reads input:id.
func (s Status) Input(id int) (v InputStatus, ok bool) {
	return v, s.Decode(idKey("input", id), &v)
}

// EM reads em:id.
func (s Status) EM(id int) (v EMStatus, ok bool) { return v, s.Decode(idKey("em", id), &v) }

// EMData reads emdata:id.
func (s Status) EMData(id int) (v EMDataStatus, ok bool) {
	return v, s.Decode(idKey("emdata", id), &v)
}

// EM1 reads em1:id.
func (s Status) EM1(id int) (v EM1Status, ok bool) { return v, s.Decode(idKey("em1", id), &v) }

// EM1Data reads em1data:id.
func (s Status) EM1Data(id int) (v EM1DataStatus, ok bool) {
	return v, s.Decode(idKey("em1data", id), &v)
}

// PM1 reads pm1:id.
func (s Status) PM1(id int) (v PM1Status, ok bool) { return v, s.Decode(idKey("pm1", id), &v) }

// Temperature reads temperature:id.
func (s Status) Temperature(id int) (v TemperatureStatus, ok bool) {
	return v, s.Decode(idKey("temperature", id), &v)
}

// Humidity reads humidity:id.
func (s Status) Humidity(id int) (v HumidityStatus, ok bool) {
	return v, s.Decode(idKey("humidity", id), &v)
}

// DevicePower reads devicepower:id.
func (s Status) DevicePower(id int) (v DevicePowerStatus, ok bool) {
	return v, s.Decode(idKey("devicepower", id), &v)
}

// Sys reads sys.
func (s Status) Sys() (v SysStatus, ok bool) { return v, s.Decode("sys", &v) }

// WiFi reads wifi.
func (s Status) WiFi() (v WiFiStatus, ok bool) { return v, s.Decode("wifi", &v) }

// SwitchConfig is a switch component's configuration. InMode is how the
// input drives it (momentary, follow, flip, detached, cycle, activate);
// InitialState is on, off, restore_last or match_input.
type SwitchConfig struct {
	ID               int      `json:"id"`
	Name             *string  `json:"name"`
	InMode           string   `json:"in_mode,omitempty"`
	InitialState     string   `json:"initial_state,omitempty"`
	AutoOn           bool     `json:"auto_on"`
	AutoOnDelay      float64  `json:"auto_on_delay,omitempty"`
	AutoOff          bool     `json:"auto_off"`
	AutoOffDelay     float64  `json:"auto_off_delay,omitempty"`
	PowerLimit       *float64 `json:"power_limit,omitempty"`
	VoltageLimit     *float64 `json:"voltage_limit,omitempty"`
	CurrentLimit     *float64 `json:"current_limit,omitempty"`
	AutorecoverVolts *bool    `json:"autorecover_voltage_errors,omitempty"`
}

// CoverConfig is a cover component's configuration (the common fields).
type CoverConfig struct {
	ID              int      `json:"id"`
	Name            *string  `json:"name"`
	InMode          string   `json:"in_mode,omitempty"`
	InitialState    string   `json:"initial_state,omitempty"`
	InvertDirection bool     `json:"invert_directions"`
	SwapInputs      bool     `json:"swap_inputs"`
	MaxtimeOpen     *float64 `json:"maxtime_open,omitempty"`
	MaxtimeClose    *float64 `json:"maxtime_close,omitempty"`
	PowerLimit      *float64 `json:"power_limit,omitempty"`
}

// InputConfig is an input component's configuration. Type is switch,
// button, analog or count.
type InputConfig struct {
	ID           int     `json:"id"`
	Name         *string `json:"name"`
	Type         string  `json:"type"`
	Enable       *bool   `json:"enable,omitempty"`
	Invert       bool    `json:"invert"`
	FactoryReset *bool   `json:"factory_reset,omitempty"`
}

// LightConfig is the configuration of a light, rgb, rgbw or cct
// component (the common fields).
type LightConfig struct {
	ID            int      `json:"id"`
	Name          *string  `json:"name"`
	InMode        string   `json:"in_mode,omitempty"`
	InitialState  string   `json:"initial_state,omitempty"`
	AutoOn        bool     `json:"auto_on"`
	AutoOnDelay   float64  `json:"auto_on_delay,omitempty"`
	AutoOff       bool     `json:"auto_off"`
	AutoOffDelay  float64  `json:"auto_off_delay,omitempty"`
	TransitionDur *float64 `json:"transition_duration,omitempty"`
	Default       *struct {
		Brightness *float64 `json:"brightness"`
	} `json:"default,omitempty"`
	NightMode *struct {
		Enable     bool     `json:"enable"`
		Brightness *float64 `json:"brightness"`
	} `json:"night_mode,omitempty"`
}

// SysConfig is the sys component's configuration: the device block
// (name, MAC, firmware id, discoverability), location and revision.
type SysConfig struct {
	Device struct {
		Name         *string `json:"name"`
		MAC          string  `json:"mac"`
		FWID         string  `json:"fw_id"`
		Discoverable bool    `json:"discoverable"`
		EcoMode      bool    `json:"eco_mode"`
		Profile      string  `json:"profile,omitempty"`
	} `json:"device"`
	Location struct {
		TZ  *string  `json:"tz"`
		Lat *float64 `json:"lat"`
		Lon *float64 `json:"lon"`
	} `json:"location"`
	CfgRev int `json:"cfg_rev"`
}

// WiFiConfigView is the wifi component's configuration (passwords are
// never returned by devices).
type WiFiConfigView struct {
	AP struct {
		SSID   *string `json:"ssid"`
		Enable bool    `json:"enable"`
		IsOpen bool    `json:"is_open"`
	} `json:"ap"`
	STA struct {
		SSID   *string `json:"ssid"`
		Enable bool    `json:"enable"`
		IsOpen bool    `json:"is_open"`
		IPv4   string  `json:"ipv4mode,omitempty"`
	} `json:"sta"`
	STA1 struct {
		SSID   *string `json:"ssid"`
		Enable bool    `json:"enable"`
	} `json:"sta1"`
}

// Switch reads switch:id's configuration.
func (c Config) Switch(id int) (v SwitchConfig, ok bool) {
	return v, c.Decode(idKey("switch", id), &v)
}

// Cover reads cover:id's configuration.
func (c Config) Cover(id int) (v CoverConfig, ok bool) {
	return v, c.Decode(idKey("cover", id), &v)
}

// Input reads input:id's configuration.
func (c Config) Input(id int) (v InputConfig, ok bool) {
	return v, c.Decode(idKey("input", id), &v)
}

// Light reads a light-like component's configuration (typ light, rgb,
// rgbw or cct).
func (c Config) Light(typ string, id int) (v LightConfig, ok bool) {
	return v, c.Decode(idKey(typ, id), &v)
}

// Sys reads sys's configuration.
func (c Config) Sys() (v SysConfig, ok bool) { return v, c.Decode("sys", &v) }

// WiFi reads wifi's configuration.
func (c Config) WiFi() (v WiFiConfigView, ok bool) { return v, c.Decode("wifi", &v) }

// Name is the device name in sys's configuration ("" when none is set).
func (c Config) Name() string {
	if s, ok := c.Sys(); ok && s.Device.Name != nil {
		return *s.Device.Name
	}
	return ""
}
