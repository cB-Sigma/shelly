package shelly

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Shelly devices can report to an MQTT broker instead of (Gen1: often
// instead of) answering on the LAN, as Shelly's API documents describe:
//
//   - Gen1 publishes under shellies/<device id>/: relay/<n> ("on", "off"),
//     relay/<n>/power (W), relay/<n>/energy (watt-minutes), light/<n>
//     and light/<n>/status (JSON), roller/<n> and roller/<n>/pos,
//     input_event/<n> ({"event", "event_cnt"}), sensor/temperature,
//     sensor/humidity, sensor/battery, online ("true"/"false"), and
//     announces itself on shellies/announce ({"id", "model", "mac", "ip"}).
//     It takes relay/<n>/command, light/<n>/command and light/<n>/set,
//     roller/<n>/command and roller/<n>/command/pos.
//   - Gen2+ publishes JSON-RPC notifications on <prefix>/events/rpc (the
//     prefix is the device id unless set), and online on <prefix>/online;
//     it takes JSON-RPC requests on <prefix>/rpc and answers on
//     <src>/rpc.
//
// MQTT is the protocol side of both; the application brings the MQTT
// client: it subscribes to Filters, hands every message to Handle, and
// gives MQTT a Publisher to send with.

// DefaultMQTTPrefix is the Gen1 topic root devices use unless set.
const DefaultMQTTPrefix = "shellies"

// Publisher publishes MQTT messages: the application's MQTT client.
type Publisher interface {
	Publish(ctx context.Context, topic string, payload []byte, retain bool) error
}

// MQTT reads and commands devices in MQTT mode over an application's MQTT
// client. Set the fields before use; it is safe for concurrent use after.
type MQTT struct {
	// Prefix is the Gen1 topic root (default DefaultMQTTPrefix).
	Prefix string
	// Src is the JSON-RPC "src" of requests; devices answer on <Src>/rpc.
	Src string
	// Timeout bounds the wait for a Gen2+ answer (default 5 s).
	Timeout time.Duration
	// Pub sends the requests and commands.
	Pub Publisher

	mu      sync.Mutex
	waiters map[int]chan rpcAnswer
	next    int
}

type rpcAnswer struct {
	result json.RawMessage
	err    *RPCError
}

func (m *MQTT) prefix() string {
	if m.Prefix == "" {
		return DefaultMQTTPrefix
	}
	return m.Prefix
}

// Filters are the topic filters to subscribe to: everything under the
// Gen1 root, Gen2+ notifications and online states, and the answers to
// this client's requests.
func (m *MQTT) Filters() []string {
	return []string{m.prefix() + "/#", "+/events/rpc", "+/online", m.Src + "/rpc"}
}

// MQTTKind says what an MQTT message is.
type MQTTKind int

// The kinds of message Handle returns.
const (
	// MQTTNone is a message that is not for the application: an answer
	// to a request (delivered to RPC), or one Handle cannot read.
	MQTTNone MQTTKind = iota
	// MQTTNotification is a Gen2+ JSON-RPC notification.
	MQTTNotification
	// MQTTAnnounce is a Gen1 device's announcement.
	MQTTAnnounce
	// MQTTGen1 is a Gen1 device's topic.
	MQTTGen1
)

// Announce is a Gen1 device's announcement on <prefix>/announce.
type Announce struct {
	ID    string // the device id (shellyplug-s-98CDAC1F0A0A)
	Model string // the type code (SHPLG-S)
	MAC   string // 12 hexadecimal digits
	IP    string // the device's address
}

// MQTTMessage is a message Handle read.
type MQTTMessage struct {
	// Kind says which of the fields below are set.
	Kind MQTTKind
	// Root is the device's topic root: the Gen2+ prefix before
	// /events/rpc, or <Prefix>/<id> for Gen1 and announcements.
	Root string
	// Src, Method and Params are a Gen2+ notification's.
	Src, Method string
	Params      json.RawMessage
	// Announce is a Gen1 announcement.
	Announce Announce
	// Device is a Gen1 device's id, Topic its topic below <Prefix>/<id>/
	// (split at "/") and Report what that topic says.
	Device string
	Topic  []string
	Report Gen1Report
}

// Handle reads one received message. Answers to RPC requests are
// delivered to the waiting call and come back as MQTTNone.
func (m *MQTT) Handle(topic string, payload []byte) MQTTMessage {
	parts := strings.Split(topic, "/")
	prefix := m.prefix()
	switch {
	case len(parts) == 2 && parts[0] == m.Src && parts[1] == "rpc":
		var r struct {
			ID     int             `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *RPCError       `json:"error"`
		}
		if json.Unmarshal(payload, &r) == nil {
			m.mu.Lock()
			ch := m.waiters[r.ID]
			delete(m.waiters, r.ID)
			m.mu.Unlock()
			if ch != nil {
				ch <- rpcAnswer{result: r.Result, err: r.Error}
			}
		}
	case len(parts) >= 3 && parts[len(parts)-2] == "events" && parts[len(parts)-1] == "rpc":
		var f struct {
			Src    string          `json:"src"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(payload, &f) != nil || f.Src == "" {
			return MQTTMessage{}
		}
		return MQTTMessage{Kind: MQTTNotification, Root: strings.Join(parts[:len(parts)-2], "/"), Src: f.Src, Method: f.Method, Params: f.Params}
	case parts[0] == prefix && len(parts) >= 2 && parts[1] == "announce":
		var a Announce
		if json.Unmarshal(payload, &a) == nil && a.ID != "" {
			return MQTTMessage{Kind: MQTTAnnounce, Root: prefix + "/" + a.ID, Announce: a}
		}
	case parts[0] == prefix && len(parts) >= 3:
		return MQTTMessage{Kind: MQTTGen1, Root: prefix + "/" + parts[1], Device: parts[1], Topic: parts[2:], Report: ParseGen1Report(parts[2:], string(payload))}
	}
	return MQTTMessage{}
}

// Gen1TopicKind says what a Gen1 status topic reports.
type Gen1TopicKind int

// The Gen1 status topics.
const (
	// TopicOther is a topic not listed here (info, announce, …).
	TopicOther Gen1TopicKind = iota
	// TopicOnline is online: Online.
	TopicOnline
	// TopicRelay is relay/<n>: On.
	TopicRelay
	// TopicRelayPower is relay/<n>/power in W: Num.
	TopicRelayPower
	// TopicRelayEnergy is relay/<n>/energy in watt-minutes: Num.
	TopicRelayEnergy
	// TopicLight is light/<n>: On.
	TopicLight
	// TopicLightStatus is light/<n>/status: Light (nil when not JSON).
	TopicLightStatus
	// TopicRoller is roller/<n>: Text, the direction of travel (open,
	// close, stop).
	TopicRoller
	// TopicRollerPos is roller/<n>/pos in percent: Num (-1 while not
	// calibrated).
	TopicRollerPos
	// TopicInputEvent is input_event/<n>: Event and EventCnt.
	TopicInputEvent
	// TopicSensor is sensor/<name>: Name and Num.
	TopicSensor
)

// Gen1Report is what one Gen1 topic says.
type Gen1Report struct {
	// Kind says which fields are set.
	Kind Gen1TopicKind
	// Index is the output or input number; 0 when the topic has none.
	Index int
	// Text is the payload as sent.
	Text string
	// On is the state of relay/<n> and light/<n> ("on").
	On bool
	// Online is the state of online ("true").
	Online bool
	// Num is a number payload; IsNum is false when it was not a number.
	Num   float64
	IsNum bool
	// Light is light/<n>/status (ison, brightness, mode, …).
	Light map[string]any
	// Event and EventCnt are an input_event's ("S", "L", "SS"; Gen1Event
	// names them).
	Event    string
	EventCnt float64
	// Name is a sensor's name (temperature, humidity, battery, lux).
	Name string
}

// ParseGen1Report reads a Gen1 topic below shellies/<id>/ (split at "/")
// and its payload. An input_event without an event, and a sensor or meter
// that is not a number, keep their Kind with the fields unset.
func ParseGen1Report(rest []string, payload string) Gen1Report {
	r := Gen1Report{Text: payload}
	if len(rest) == 0 {
		return r
	}
	if len(rest) >= 2 {
		if n, err := strconv.Atoi(rest[1]); err == nil {
			r.Index = n
		}
	}
	num := func() {
		f, err := strconv.ParseFloat(strings.TrimSpace(payload), 64)
		r.Num, r.IsNum = f, err == nil
	}
	switch {
	case rest[0] == "online":
		r.Kind, r.Online = TopicOnline, payload == "true"
	case rest[0] == "relay" && len(rest) == 2:
		r.Kind, r.On = TopicRelay, payload == "on"
	case rest[0] == "relay" && len(rest) == 3 && rest[2] == "power":
		r.Kind = TopicRelayPower
		num()
	case rest[0] == "relay" && len(rest) == 3 && rest[2] == "energy":
		r.Kind = TopicRelayEnergy
		num()
	case rest[0] == "light" && len(rest) == 2:
		r.Kind, r.On = TopicLight, payload == "on"
	case rest[0] == "light" && len(rest) == 3 && rest[2] == "status":
		r.Kind = TopicLightStatus
		var s map[string]any
		if json.Unmarshal([]byte(payload), &s) == nil {
			r.Light = s
		}
	case rest[0] == "roller" && len(rest) == 2:
		r.Kind = TopicRoller
	case rest[0] == "roller" && len(rest) == 3 && rest[2] == "pos":
		r.Kind = TopicRollerPos
		num()
	case rest[0] == "input_event" && len(rest) == 2:
		r.Kind = TopicInputEvent
		var e struct {
			Event string  `json:"event"`
			Cnt   float64 `json:"event_cnt"`
		}
		if json.Unmarshal([]byte(payload), &e) == nil {
			r.Event, r.EventCnt = e.Event, e.Cnt
		}
	case rest[0] == "sensor" && len(rest) == 2:
		r.Kind, r.Name = TopicSensor, rest[1]
		num()
	}
	return r
}

// MACOfID is the MAC part of a Gen1 id in lower case
// (shelly1-A4CF12F45B2E → a4cf12f45b2e); "" when the id has none.
func MACOfID(id string) string {
	if i := strings.LastIndex(id, "-"); i >= 0 && len(id)-i-1 >= 6 {
		return strings.ToLower(id[i+1:])
	}
	return ""
}

// RPC calls a Gen2+ method over the broker: the request goes to
// <root>/rpc and the answer comes back through Handle. An error object
// in the answer is an error naming the method.
func (m *MQTT) RPC(ctx context.Context, root, method string, params map[string]any) (json.RawMessage, error) {
	m.mu.Lock()
	if m.waiters == nil {
		m.waiters = map[int]chan rpcAnswer{}
	}
	m.next++
	id := m.next
	ch := make(chan rpcAnswer, 1)
	m.waiters[id] = ch
	m.mu.Unlock()
	drop := func() {
		m.mu.Lock()
		delete(m.waiters, id)
		m.mu.Unlock()
	}
	b, _ := json.Marshal(map[string]any{"id": id, "src": m.Src, "method": method, "params": params})
	if err := m.Pub.Publish(ctx, root+"/rpc", b, false); err != nil {
		drop()
		return nil, err
	}
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("the device refused %s: %w", method, r.err)
		}
		return r.result, nil
	case <-t.C:
		drop()
		return nil, fmt.Errorf("the device did not answer %s on the MQTT broker", method)
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	}
}

// Command1 sends a Gen1 GET (path with query, as the Gen1* request
// builders make it) as the MQTT command under root (<prefix>/<id>).
// Gen1 devices do not answer commands; their next state message confirms.
func (m *MQTT) Command1(ctx context.Context, root, pathQuery string) error {
	topic, payload, err := Gen1MQTT(pathQuery)
	if err != nil {
		return err
	}
	return m.Pub.Publish(ctx, root+"/"+topic, []byte(payload), false)
}

// Gen1MQTT turns a Gen1 GET into the MQTT command topic (under
// shellies/<id>/) and payload.
func Gen1MQTT(pathQuery string) (string, string, error) {
	u, err := url.Parse(pathQuery)
	if err != nil {
		return "", "", err
	}
	q := u.Query()
	p := strings.Trim(u.Path, "/")
	parts := strings.Split(p, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("%s has no MQTT form", pathQuery)
	}
	kind, n := parts[0], parts[1]
	switch kind {
	case "relay":
		return "relay/" + n + "/command", q.Get("turn"), nil
	case "light", "color", "white":
		if len(q) == 1 && q.Get("turn") != "" {
			return "light/" + n + "/command", q.Get("turn"), nil
		}
		set := map[string]any{}
		for k := range q {
			if f, err := strconv.ParseFloat(q.Get(k), 64); err == nil {
				set[k] = f
			} else {
				set[k] = q.Get(k)
			}
		}
		b, _ := json.Marshal(set)
		return kind + "/" + n + "/set", string(b), nil
	case "roller":
		switch q.Get("go") {
		case "open", "close", "stop":
			return "roller/" + n + "/command", q.Get("go"), nil
		case "to_pos":
			return "roller/" + n + "/command/pos", q.Get("roller_pos"), nil
		}
	}
	return "", "", fmt.Errorf("%s has no MQTT form", pathQuery)
}
