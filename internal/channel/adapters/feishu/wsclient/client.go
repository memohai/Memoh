// Package wsclient is a Feishu/Lark long-connection websocket client.
//
// We use it instead of larkws.Client from the upstream SDK
// (github.com/larksuite/oapi-sdk-go/v3), whose Start blocks at select{},
// ignores ctx cancel, and never closes the websocket (upstream #141,
// #204). With the SDK version, every "save" in the channel-config UI
// leaves the old TCP conn alive while Feishu keeps routing events to it.
// Saves accumulate stale conns until the per-app limit is hit.
//
// Only the transport is replaced. Event payloads still go through
// dispatcher.EventDispatcher (passed in as the EventDispatcher
// interface), frames reuse larkws.Frame, and outbound messages still
// use lark.Client.
//
// A Client is single-shot. Run blocks until ctx is canceled or the
// websocket breaks; reconnection is the caller's job.
package wsclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"
)

// EventDispatcher is the subset of dispatcher.EventDispatcher we use.
// The SDK type satisfies it directly.
type EventDispatcher interface {
	Do(ctx context.Context, payload []byte) (interface{}, error)
}

// Config configures a Client. AppID, AppSecret and Domain are required.
type Config struct {
	AppID     string
	AppSecret string
	// Domain is the Feishu/Lark open API base, e.g. https://open.feishu.cn.
	Domain string
	// Logger is optional; defaults to slog.Default.
	Logger *slog.Logger

	// PingInterval is the local ping cadence. The server can override
	// it via the ClientConfig returned by the endpoint handshake.
	// Default: 2 minutes.
	PingInterval time.Duration
	// HandshakeTimeout caps the websocket handshake. Default: 30s.
	HandshakeTimeout time.Duration
	// CloseGracePeriod caps how long Run waits for the server to ack a
	// graceful close frame before forcing the underlying TCP connection
	// shut. Default: 2s.
	CloseGracePeriod time.Duration
}

// Client is a single-shot Feishu websocket transport.
type Client struct {
	cfg    Config
	logger *slog.Logger
}

// buildDialer returns a Dialer copy of websocket.DefaultDialer so HTTP_PROXY
// / HTTPS_PROXY / NO_PROXY env vars apply via http.ProxyFromEnvironment.
func (c *Client) buildDialer() *websocket.Dialer {
	d := *websocket.DefaultDialer
	d.HandshakeTimeout = c.cfg.HandshakeTimeout
	return &d
}

// New creates a Client. Run can only be called once; create a fresh
// instance for each connection attempt.
func New(cfg Config) *Client {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.PingInterval <= 0 {
		cfg.PingInterval = 2 * time.Minute
	}
	if cfg.HandshakeTimeout <= 0 {
		cfg.HandshakeTimeout = 30 * time.Second
	}
	if cfg.CloseGracePeriod <= 0 {
		cfg.CloseGracePeriod = 2 * time.Second
	}
	return &Client{cfg: cfg, logger: logger.With(slog.String("component", "feishu_ws"))}
}

// Run establishes the websocket, dispatches inbound events, and blocks
// until ctx is canceled or the connection breaks.
//
// On ctx cancel Run does a best-effort close handshake and returns nil.
// On websocket failure it returns the underlying error.
func (c *Client) Run(ctx context.Context, dispatcher EventDispatcher) error {
	if dispatcher == nil {
		return errors.New("feishu wsclient: dispatcher is required")
	}

	endpoint, serverCfg, err := c.fetchEndpoint(ctx)
	if err != nil {
		return fmt.Errorf("fetch endpoint: %w", err)
	}

	pingInterval := c.cfg.PingInterval
	if serverCfg != nil && serverCfg.PingInterval > 0 {
		pingInterval = time.Duration(serverCfg.PingInterval) * time.Second
	}

	connURL, err := url.Parse(endpoint.Url)
	if err != nil {
		return fmt.Errorf("parse endpoint url: %w", err)
	}
	serviceID := connURL.Query().Get(larkws.ServiceID)
	connID := connURL.Query().Get(larkws.DeviceID)

	dialer := c.buildDialer()
	conn, resp, err := dialer.DialContext(ctx, endpoint.Url, nil)
	if err != nil {
		if resp != nil {
			if hsErr := parseHandshakeError(resp); hsErr != nil {
				return hsErr
			}
		}
		return fmt.Errorf("ws dial: %w", err)
	}
	if resp != nil {
		_ = resp.Body.Close()
	}

	c.logger.Info("connected",
		slog.String("conn_id", connID),
		slog.String("service_id", serviceID),
		slog.Duration("ping_interval", pingInterval),
	)

	session := &session{
		client:        c,
		conn:          conn,
		dispatcher:    dispatcher,
		serviceID:     serviceID,
		connID:        connID,
		pingInterval:  pingInterval,
		fragmentCache: newFragmentCache(5 * time.Second),
	}
	defer session.fragmentCache.stop()
	return session.run(ctx)
}

// session encapsulates the per-connection state so it can be torn down
// independently of the long-lived Client.
type session struct {
	client     *Client
	conn       *websocket.Conn
	dispatcher EventDispatcher

	serviceID string
	connID    string

	writeMu      sync.Mutex
	pingInterval time.Duration

	fragmentCache *fragmentCache

	closeOnce sync.Once
}

func (s *session) run(ctx context.Context) error {
	// sessionCtx is canceled by either the caller (via ctx) or by
	// readLoop returning an error. Routing both through one cancel
	// makes pingLoop and the close goroutine exit on whichever path
	// fires first.
	sessionCtx, cancelSession := context.WithCancel(ctx)
	defer cancelSession()

	// Bind the underlying conn close to sessionCtx cancellation. Closing
	// the conn unblocks ReadMessage and frees the OS socket immediately.
	closeDone := make(chan struct{})
	go func() {
		defer close(closeDone)
		<-sessionCtx.Done()
		s.gracefulClose()
	}()

	pingDone := make(chan struct{})
	go func() {
		defer close(pingDone)
		s.pingLoop(sessionCtx)
	}()

	readErr := s.readLoop(sessionCtx)

	// Drain pingLoop and the close goroutine so run never returns
	// with helpers still in flight.
	cancelSession()
	<-pingDone
	<-closeDone

	if ctx.Err() != nil {
		// Caller canceled; treat any read-side error as the expected
		// "closed connection" noise.
		return nil
	}
	return readErr
}

func (s *session) gracefulClose() {
	s.closeOnce.Do(func() {
		grace := s.client.cfg.CloseGracePeriod
		deadline := time.Now().Add(grace)

		s.writeMu.Lock()
		_ = s.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			deadline,
		)
		s.writeMu.Unlock()
		_ = s.conn.Close()

		s.client.logger.Info("disconnected",
			slog.String("conn_id", s.connID),
			slog.String("service_id", s.serviceID),
		)
	})
}

func (s *session) readLoop(ctx context.Context) error {
	for {
		mt, msg, err := s.conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("ws read: %w", err)
		}
		if mt != websocket.BinaryMessage {
			s.client.logger.Warn("ignoring non-binary frame", slog.Int("type", mt))
			continue
		}
		s.handleFrame(ctx, msg)
	}
}

func (s *session) pingLoop(ctx context.Context) {
	ticker := time.NewTicker(s.pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.sendPing(); err != nil {
				if ctx.Err() != nil {
					return
				}
				s.client.logger.Warn("ping failed", slog.Any("error", err))
			}
		}
	}
}

func (s *session) sendPing() error {
	sid, _ := strconv.ParseInt(s.serviceID, 10, 32)
	frame := larkws.NewPingFrame(int32(sid))
	bs, err := frame.Marshal()
	if err != nil {
		return fmt.Errorf("marshal ping: %w", err)
	}
	return s.writeBinary(bs)
}

func (s *session) writeBinary(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (s *session) handleFrame(ctx context.Context, raw []byte) {
	var frame larkws.Frame
	if err := frame.Unmarshal(raw); err != nil {
		s.client.logger.Error("unmarshal frame failed", slog.Any("error", err))
		return
	}

	switch larkws.FrameType(frame.Method) {
	case larkws.FrameTypeControl:
		s.handleControl(frame)
	case larkws.FrameTypeData:
		// dispatcher.Do can run slow HTTP enrichment in the user
		// handler; off-loading keeps it from stalling the next read
		// and the server's pong.
		go s.handleData(ctx, frame)
	default:
		s.client.logger.Debug("unknown frame method", slog.Int("method", int(frame.Method)))
	}
}

func (s *session) handleControl(frame larkws.Frame) {
	headers := larkws.Headers(frame.Headers)
	t := headers.GetString(larkws.HeaderType)
	if larkws.MessageType(t) != larkws.MessageTypePong {
		return
	}
	if len(frame.Payload) == 0 {
		return
	}
	cfg := &larkws.ClientConfig{}
	if err := json.Unmarshal(frame.Payload, cfg); err != nil {
		s.client.logger.Warn("decode pong client config failed", slog.Any("error", err))
		return
	}
	if cfg.PingInterval > 0 {
		next := time.Duration(cfg.PingInterval) * time.Second
		if next != s.pingInterval {
			s.client.logger.Debug("server adjusted ping interval",
				slog.Duration("old", s.pingInterval),
				slog.Duration("new", next),
			)
			s.pingInterval = next
			// The running ticker keeps the old cadence until the
			// next reconnect; re-arming mid-flight isn't worth the
			// extra plumbing.
		}
	}
}

func (s *session) handleData(ctx context.Context, frame larkws.Frame) {
	headers := larkws.Headers(frame.Headers)
	sum := headers.GetInt(larkws.HeaderSum)
	seq := headers.GetInt(larkws.HeaderSeq)
	msgID := headers.GetString(larkws.HeaderMessageID)
	traceID := headers.GetString(larkws.HeaderTraceID)
	msgType := headers.GetString(larkws.HeaderType)

	payload := frame.Payload
	if sum > 1 {
		payload = s.fragmentCache.add(msgID, sum, seq, payload)
		if payload == nil {
			return
		}
	}

	switch larkws.MessageType(msgType) {
	case larkws.MessageTypeEvent:
	case larkws.MessageTypeCard:
		// Card callbacks aren't wired up.
		return
	default:
		return
	}

	startMs := time.Now().UnixMilli()
	rsp, err := s.dispatcher.Do(ctx, payload)
	endMs := time.Now().UnixMilli()
	headers.Add(larkws.HeaderBizRt, strconv.FormatInt(endMs-startMs, 10))

	resp := larkws.NewResponseByCode(http.StatusOK)
	switch {
	case err != nil:
		s.client.logger.Error("dispatch event failed",
			slog.String("message_id", msgID),
			slog.String("trace_id", traceID),
			slog.Any("error", err),
		)
		resp = larkws.NewResponseByCode(http.StatusInternalServerError)
	case rsp != nil:
		data, encErr := json.Marshal(rsp)
		if encErr != nil {
			s.client.logger.Error("encode dispatch response failed",
				slog.String("message_id", msgID),
				slog.String("trace_id", traceID),
				slog.Any("error", encErr),
			)
			resp = larkws.NewResponseByCode(http.StatusInternalServerError)
		} else {
			resp.Data = data
		}
	}

	encoded, _ := json.Marshal(resp)
	frame.Payload = encoded
	frame.Headers = headers
	bs, marshalErr := frame.Marshal()
	if marshalErr != nil {
		s.client.logger.Error("marshal response frame failed", slog.Any("error", marshalErr))
		return
	}
	if writeErr := s.writeBinary(bs); writeErr != nil {
		if ctx.Err() != nil {
			return
		}
		s.client.logger.Error("write response frame failed",
			slog.String("message_id", msgID),
			slog.String("trace_id", traceID),
			slog.Any("error", writeErr),
		)
	}
}

// fetchEndpoint mirrors the upstream SDK's getConnURL: it asks Feishu for a
// websocket endpoint URL plus an optional ClientConfig (ping/reconnect
// hints).
func (c *Client) fetchEndpoint(ctx context.Context) (*larkws.Endpoint, *larkws.ClientConfig, error) {
	body, err := json.Marshal(map[string]string{
		"AppID":     c.cfg.AppID,
		"AppSecret": c.cfg.AppSecret,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("encode endpoint request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Domain+larkws.GenEndpointUri, bytes.NewBuffer(body))
	if err != nil {
		return nil, nil, fmt.Errorf("build endpoint request: %w", err)
	}
	req.Header.Add("locale", "zh")
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // G107: domain comes from Config (lark/feishu base URL), not user input
	if err != nil {
		return nil, nil, fmt.Errorf("call endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("read endpoint response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, larkws.NewServerError(resp.StatusCode, "endpoint request failed: "+string(raw))
	}
	parsed := &larkws.EndpointResp{}
	if err := json.Unmarshal(raw, parsed); err != nil {
		return nil, nil, fmt.Errorf("decode endpoint response: %w", err)
	}
	switch parsed.Code {
	case larkws.OK:
	case larkws.SystemBusy:
		return nil, nil, larkws.NewServerError(parsed.Code, "system busy")
	case larkws.InternalError:
		return nil, nil, larkws.NewServerError(parsed.Code, parsed.Msg)
	default:
		return nil, nil, larkws.NewClientError(parsed.Code, parsed.Msg)
	}
	if parsed.Data == nil || parsed.Data.Url == "" {
		return nil, nil, larkws.NewServerError(http.StatusInternalServerError, "endpoint url is empty")
	}
	return parsed.Data, parsed.Data.ClientConfig, nil
}

// parseHandshakeError mirrors the upstream SDK's parseErr: it converts the
// HTTP response from a failed websocket upgrade into a typed Server/Client
// error so callers can distinguish recoverable vs terminal failures.
func parseHandshakeError(resp *http.Response) error {
	if resp == nil {
		return nil
	}
	statusStr := resp.Header.Get(larkws.HeaderHandshakeStatus)
	msg := resp.Header.Get(larkws.HeaderHandshakeMsg)
	if statusStr == "" {
		return nil
	}
	code, _ := strconv.Atoi(statusStr)
	switch code {
	case larkws.AuthFailed:
		authStr := resp.Header.Get(larkws.HeaderHandshakeAuthErrCode)
		authCode, _ := strconv.Atoi(authStr)
		if authCode == larkws.ExceedConnLimit {
			return larkws.NewClientError(code, msg)
		}
		return larkws.NewServerError(code, msg)
	case larkws.Forbidden:
		return larkws.NewClientError(code, msg)
	default:
		return larkws.NewServerError(code, msg)
	}
}
