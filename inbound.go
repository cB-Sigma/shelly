package shelly

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

// Battery-powered Gen2+ devices (Plus H&T, door/window sensors) sleep and
// wake only to report, so nobody can open a channel to them. They can be
// told to open one themselves: the "outbound WebSocket" (Ws component;
// Ws.SetConfig with enable and server, see Client.SetOutboundWS). Inbound
// accepts these channels and reads the frames the devices send:
// NotifyFullStatus on connect, NotifyStatus and NotifyEvent after, each
// with the device's id as "src".

// Frame is one notification a device sent on its outbound WebSocket.
type Frame struct {
	// Src is the device's id.
	Src string
	// Method is NotifyFullStatus, NotifyStatus or NotifyEvent.
	Method string
	// Params are the notification's params as sent.
	Params json.RawMessage
}

// InboundChannel is one device's outbound WebSocket.
type InboundChannel struct {
	// RemoteIP is the device's address.
	RemoteIP string
	// Value is the application's own state for the channel (the device
	// it belongs to, once known); Inbound does not touch it.
	Value any
}

// Inbound is an http.Handler that accepts devices' outbound WebSockets.
// Mount it at the path devices are told (Ws.SetConfig's server), or run
// it with ServeInbound.
type Inbound struct {
	// Frame is called for each frame that has a src and a method, in the
	// order the channel's frames arrive.
	Frame func(ctx context.Context, ch *InboundChannel, f Frame)
	// Closed, when set, is called once a channel has ended.
	Closed func(ch *InboundChannel)
}

// ServeHTTP accepts one channel and reads its frames until it closes or
// the request's context ends.
func (in *Inbound) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	defer func() { _ = c.CloseNow() }()
	c.SetReadLimit(1 << 20)
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	ch := &InboundChannel{RemoteIP: host}
	ctx := r.Context()
	for {
		_, msg, err := c.Read(ctx)
		if err != nil {
			if in.Closed != nil {
				in.Closed(ch)
			}
			return
		}
		var f Frame
		if json.Unmarshal(msg, &f) != nil || f.Src == "" || f.Method == "" {
			continue
		}
		if in.Frame != nil {
			in.Frame(ctx, ch, f)
		}
	}
}

// ServeInbound serves in at path on ln until ctx ends, which also ends the
// open channels; it closes ln.
func ServeInbound(ctx context.Context, ln net.Listener, path string, in *Inbound) error {
	mux := http.NewServeMux()
	mux.Handle(path, in)
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second, BaseContext: func(net.Listener) context.Context { return ctx }}
	stop := context.AfterFunc(ctx, func() { _ = srv.Close() })
	defer stop()
	err := srv.Serve(ln)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// OutboundWS is a Gen2+ device's outbound WebSocket setting (Ws.GetConfig).
type OutboundWS struct {
	// Enable is true when the device opens the channel.
	Enable bool `json:"enable"`
	// Server is the ws:// or wss:// address it connects to.
	Server string `json:"server"`
}

// OutboundWS reads the device's outbound WebSocket setting; an error when
// the device has no Ws component (older firmware).
func (c *Client) OutboundWS(ctx context.Context) (OutboundWS, error) {
	var cfg OutboundWS
	raw, err := c.RPC(ctx, "Ws.GetConfig", nil)
	if err != nil {
		return cfg, err
	}
	_ = json.Unmarshal(raw, &cfg)
	return cfg, nil
}

// SetOutboundWS tells the device to open its outbound WebSocket to server
// (any certificate authority for wss). restart is true when the device
// takes the setting only after a restart (Reboot).
func (c *Client) SetOutboundWS(ctx context.Context, server string) (restart bool, err error) {
	res, err := c.RPC(ctx, "Ws.SetConfig", map[string]any{"config": map[string]any{"enable": true, "server": server, "ssl_ca": "*"}})
	if err != nil {
		return false, err
	}
	var ans struct {
		Restart bool `json:"restart_required"`
	}
	_ = json.Unmarshal(res, &ans)
	return ans.Restart, nil
}

// Reboot restarts a Gen2+ device (Shelly.Reboot).
func (c *Client) Reboot(ctx context.Context) error {
	_, err := c.RPC(ctx, "Shelly.Reboot", nil)
	return err
}
