package shelly

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// WSOptions configure WatchWS.
type WSOptions struct {
	// Src is the "src" of the requests: the name the device answers to.
	Src string
	// Password answers the device's authentication challenge; with none,
	// a device that asks fails with ErrAuth.
	Password string
	// Timeout bounds the dial, each write and each ping (default 5 s).
	Timeout time.Duration
	// Ping is how often the channel is pinged to notice one that died
	// without a close (default a minute).
	Ping time.Duration
	// Status receives the result of the Shelly.GetStatus request the
	// channel opens with: the device's complete status, by component.
	Status func(status map[string]any) error
	// Notify receives each notification (NotifyStatus, NotifyFullStatus,
	// NotifyEvent) with its params as sent.
	Notify func(method string, params json.RawMessage) error
}

// WSURL is the WebSocket address of a device's RPC channel for a Client
// Base (http://host → ws://host/rpc, https → wss).
func WSURL(base string) string {
	return "ws" + strings.TrimPrefix(base, "http") + "/rpc"
}

// WatchWS opens the WebSocket RPC channel of a Gen2+ device at base
// (http://host[:port]), asks for Shelly.GetStatus, authenticates when the
// device asks (WSAuth), and hands the status and every notification to
// the options' functions until ctx ends, the channel fails or a function
// returns an error, which WatchWS then returns. A refused password is
// ErrAuth.
func WatchWS(ctx context.Context, base string, o WSOptions) error {
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	ping := o.Ping
	if ping <= 0 {
		ping = time.Minute
	}
	dctx, cancel := context.WithTimeout(ctx, timeout)
	c, _, err := websocket.Dial(dctx, WSURL(base), nil)
	cancel()
	if err != nil {
		return err
	}
	defer func() { _ = c.CloseNow() }()
	c.SetReadLimit(1 << 20)
	sctx, stop := context.WithCancel(ctx)
	defer stop()

	send := func(id int, auth map[string]any) error {
		req := map[string]any{"id": id, "src": o.Src, "method": "Shelly.GetStatus"}
		if auth != nil {
			req["auth"] = auth
		}
		b, _ := json.Marshal(req)
		wctx, cancel := context.WithTimeout(sctx, timeout)
		defer cancel()
		return c.Write(wctx, websocket.MessageText, b)
	}
	if err := send(1, nil); err != nil {
		return err
	}
	// Keep-alive: a ping now and then notices a channel that died
	// without a close.
	go func() {
		t := time.NewTicker(ping)
		defer t.Stop()
		for {
			select {
			case <-sctx.Done():
				return
			case <-t.C:
				pctx, cancel := context.WithTimeout(sctx, timeout)
				err := c.Ping(pctx)
				cancel()
				if err != nil {
					_ = c.CloseNow()
					return
				}
			}
		}
	}()
	authTried := false
	for {
		_, b, err := c.Read(sctx)
		if err != nil {
			return err
		}
		var f struct {
			ID     *int            `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  *RPCError       `json:"error"`
		}
		if json.Unmarshal(b, &f) != nil {
			continue
		}
		switch {
		case f.ID != nil && f.Error != nil:
			if f.Error.Code != 401 {
				return f.Error
			}
			if authTried || o.Password == "" {
				return fmt.Errorf("%w (WebSocket)", ErrAuth)
			}
			auth, err := WSAuth(f.Error.Message, o.Password)
			if err != nil {
				return err
			}
			authTried = true
			if err := send(2, auth); err != nil {
				return err
			}
		case f.ID != nil && f.Result != nil:
			var st map[string]any
			if json.Unmarshal(f.Result, &st) == nil && o.Status != nil {
				if err := o.Status(st); err != nil {
					return err
				}
			}
		case f.Method != "":
			if o.Notify != nil {
				if err := o.Notify(f.Method, f.Params); err != nil {
					return err
				}
			}
		}
	}
}

// Event is one entry of a NotifyEvent's events list.
type Event struct {
	// Component is the component that raised it ("input:0").
	Component string
	// Name is the event (single_push, long_push, …).
	Name string
	// TS is the device's time stamp in seconds since 1970; 0 when absent.
	TS float64
	// Fields are all the entry's fields.
	Fields map[string]any
}

// Events reads the events of a NotifyEvent's params.
func Events(params map[string]any) []Event {
	evs, _ := params["events"].([]any)
	var out []Event
	for _, e := range evs {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		ev := Event{Fields: m}
		ev.Component, _ = m["component"].(string)
		ev.Name, _ = m["event"].(string)
		ev.TS, _ = m["ts"].(float64)
		out = append(out, ev)
	}
	return out
}
