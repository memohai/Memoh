package display

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pion/rtp"
	sdpv3 "github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
)

const (
	TransportWebRTC      = "webrtc"
	EncoderGStreamer     = "gstreamer"
	CodecH264            = webrtc.MimeTypeH264
	CodecVP8             = webrtc.MimeTypeVP8
	gstLaunchEnv         = "MEMOH_GSTREAMER_LAUNCH"
	rtcUDPPortMinEnv     = "MEMOH_DISPLAY_WEBRTC_UDP_PORT_MIN"
	rtcUDPPortMaxEnv     = "MEMOH_DISPLAY_WEBRTC_UDP_PORT_MAX"
	rtcNATIPsEnv         = "MEMOH_DISPLAY_WEBRTC_NAT_IPS"
	forceVP8Env          = "MEMOH_DISPLAY_FORCE_VP8"
	videoPayloadTypeH264 = 102
	videoPayloadTypeVP8  = 96
	videoClockRate       = 90000
	videoFrameRate       = 15
	h264FmtpLine         = "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"
	displayProbePeriod   = 5 * time.Second
	socketProbeTimeout   = 300 * time.Millisecond
	stalePeerTTL         = 2 * time.Minute
	screenshotTimeout    = 15 * time.Second
	screenshotWidth      = 1280
	screenshotQuality    = 82
	screenshotMaxBytes   = 512 * 1024
	screenshotMIME       = "image/jpeg"
	rfbTCPAddress        = "127.0.0.1:5999"
	inputChannelLabel    = "display-input"
	cursorChannelLabel   = "display-cursor"
	rfbEncodingRaw       = 0
	rfbEncodingCopyRect  = 1
	rfbEncodingCursor    = -239
	rfbEncodingXCursor   = -240
	maxCursorDimension   = 512
	maxRFBDiscardBytes   = 256 * 1024 * 1024
)

type screenshotJPEGCandidate struct {
	width   int
	quality int
}

var (
	ErrManagerUnavailable = errors.New("manager not configured")
	ErrDisplayDisabled    = errors.New("display disabled")
	ErrDisplayUnavailable = errors.New("display server not reachable")
	ErrEncoderUnavailable = errors.New("gstreamer unavailable")
	ErrCodecUnsupported   = errors.New("no compatible video codec offered")
)

var screenshotJPEGCandidates = []screenshotJPEGCandidate{
	{quality: screenshotQuality},
	{quality: 72},
	{width: 1024, quality: 68},
	{width: 800, quality: 60},
	{width: 640, quality: 52},
	{width: 480, quality: 42},
	{width: 320, quality: 30},
}

type Workspace interface {
	BotDisplayEnabled(ctx context.Context, botID string) bool
	DisplaySocketPath(botID string) string
}

type workspaceDisplayDialer interface {
	DisplayDialContext(ctx context.Context, botID, network, address string) (net.Conn, error)
}

type Status struct {
	Enabled           bool
	Available         bool
	Running           bool
	Transport         string
	Encoder           string
	EncoderAvailable  bool
	UnavailableReason string
}

type OfferRequest struct {
	Type      string   `json:"type"`
	SDP       string   `json:"sdp"`
	SessionID string   `json:"session_id,omitempty"`
	NATIPs    []string `json:"-"`
}

type OfferResponse struct {
	Type      string `json:"type"`
	SDP       string `json:"sdp"`
	SessionID string `json:"session_id"`
}

type SessionInfo struct {
	ID        string    `json:"id"`
	Codec     string    `json:"codec"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
}

type ControlInput struct {
	Type       string
	X          int
	Y          int
	ButtonMask uint8
	Keysym     uint32
	Down       bool
}

type rtcSettings struct {
	UDPPortMin uint16
	UDPPortMax uint16
	NATIPs     []string
}

type Service struct {
	logger    *slog.Logger
	workspace Workspace

	mu       sync.Mutex
	sessions map[string]*session
	starting map[string]*sessionStart
}

type sessionStart struct {
	done chan struct{}
	sess *session
	err  error
}

func NewService(logger *slog.Logger, workspace Workspace) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		logger:    logger.With(slog.String("component", "display")),
		workspace: workspace,
		sessions:  make(map[string]*session),
		starting:  make(map[string]*sessionStart),
	}
}

func (s *Service) displayTarget(botID string) string {
	if _, ok := s.workspace.(workspaceDisplayDialer); ok {
		return "bridge-tcp://" + rfbTCPAddress
	}
	return s.workspace.DisplaySocketPath(botID)
}

func (s *Service) displayReachable(ctx context.Context, botID string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, socketProbeTimeout)
	defer cancel()
	conn, err := s.dialRFB(probeCtx, botID)
	if err != nil {
		return false
	}
	return probeRFBNoneSecurity(conn) == nil
}

func (s *Service) dialRFB(ctx context.Context, botID string) (net.Conn, error) {
	if dialer, ok := s.workspace.(workspaceDisplayDialer); ok {
		conn, err := dialer.DisplayDialContext(ctx, botID, "tcp", rfbTCPAddress)
		if err == nil {
			return conn, nil
		}
		if socketPath := strings.TrimSpace(s.workspace.DisplaySocketPath(botID)); socketPath != "" {
			if fallback, fallbackErr := dialRFBSocket(ctx, socketPath); fallbackErr == nil {
				return fallback, nil
			}
		}
		return nil, fmt.Errorf("dial workspace display %s: %w", rfbTCPAddress, err)
	}
	return dialRFBSocket(ctx, s.workspace.DisplaySocketPath(botID))
}

func dialRFBSocket(ctx context.Context, socketPath string) (net.Conn, error) {
	socketPath = strings.TrimSpace(socketPath)
	if socketPath == "" {
		return nil, ErrDisplayUnavailable
	}
	dialer := net.Dialer{Timeout: displayProbePeriod}
	return dialer.DialContext(ctx, "unix", filepath.Clean(socketPath))
}

func (s *Service) Status(ctx context.Context, botID string) Status {
	status := Status{
		Transport: TransportWebRTC,
		Encoder:   EncoderGStreamer,
	}
	if s == nil || s.workspace == nil {
		status.UnavailableReason = "manager not configured"
		return status
	}

	status.Enabled = s.workspace.BotDisplayEnabled(ctx, botID)
	gstLaunch, gstErr := resolveGSTLaunch()
	status.EncoderAvailable = gstErr == nil && strings.TrimSpace(gstLaunch) != ""

	if status.Enabled {
		status.Running = s.displayReachable(ctx, botID)
	}
	status.Available = status.Enabled && status.Running && status.EncoderAvailable
	switch {
	case !status.Enabled:
	case !status.Running:
		status.UnavailableReason = "display server not reachable"
	case !status.EncoderAvailable:
		status.UnavailableReason = "gstreamer unavailable"
	}
	return status
}

func (s *Service) Answer(ctx context.Context, botID string, req OfferRequest) (OfferResponse, error) {
	if s == nil || s.workspace == nil {
		return OfferResponse{}, ErrManagerUnavailable
	}
	if !s.workspace.BotDisplayEnabled(ctx, botID) {
		return OfferResponse{}, ErrDisplayDisabled
	}
	if strings.TrimSpace(req.SDP) == "" {
		return OfferResponse{}, errors.New("offer sdp is required")
	}
	if req.Type != "" && req.Type != "offer" {
		return OfferResponse{}, fmt.Errorf("unsupported session description type %q", req.Type)
	}

	if !s.displayReachable(ctx, botID) {
		return OfferResponse{}, fmt.Errorf("%w: %s", ErrDisplayUnavailable, s.displayTarget(botID))
	}
	gstLaunch, err := resolveGSTLaunch()
	if err != nil {
		return OfferResponse{}, errors.Join(ErrEncoderUnavailable, err)
	}

	codec, err := negotiateCodec(req.SDP, forceVP8FromEnv())
	if err != nil {
		return OfferResponse{}, err
	}

	sess, err := s.session(ctx, botID, gstLaunch, codec)
	if err != nil {
		return OfferResponse{}, err
	}
	return sess.answer(ctx, req)
}

func (s *Service) ListSessions(botID string) []SessionInfo {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	sess := s.sessions[botID]
	s.mu.Unlock()
	if sess == nil || sess.closed() {
		return nil
	}
	sess.closeStalePeers(time.Now())
	return sess.peerInfos()
}

func (s *Service) CloseSession(botID, sessionID string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	sess := s.sessions[botID]
	s.mu.Unlock()
	if sess == nil || sess.closed() {
		return false
	}
	return sess.closePeer(sessionID)
}

func (s *Service) Screenshot(ctx context.Context, botID string) ([]byte, string, error) {
	if s == nil || s.workspace == nil {
		return nil, "", ErrManagerUnavailable
	}
	if !s.workspace.BotDisplayEnabled(ctx, botID) {
		return nil, "", ErrDisplayDisabled
	}
	if !s.displayReachable(ctx, botID) {
		return nil, "", fmt.Errorf("%w: %s", ErrDisplayUnavailable, s.displayTarget(botID))
	}
	gstLaunch, err := resolveGSTLaunch()
	if err != nil {
		return nil, "", errors.Join(ErrEncoderUnavailable, err)
	}

	output, err := os.CreateTemp("", "memoh-display-*.jpg")
	if err != nil {
		return nil, "", err
	}
	outputPath := output.Name()
	_ = output.Close()
	defer func() { _ = os.Remove(outputPath) }()

	runCtx, cancel := context.WithTimeout(ctx, screenshotTimeout)

	listenConfig := net.ListenConfig{}
	proxy, err := listenConfig.Listen(runCtx, "tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		return nil, "", fmt.Errorf("start RFB screenshot shim: %w", err)
	}
	defer func() { _ = proxy.Close() }()
	defer cancel()
	go proxyRFBListener(runCtx, proxy, func(ctx context.Context) (net.Conn, error) {
		return s.dialRFB(ctx, botID)
	}, s.logger, botID, false)

	proxyPort := proxy.Addr().(*net.TCPAddr).Port
	cmd := exec.CommandContext(runCtx, gstLaunch, gstreamerScreenshotArgs(proxyPort, outputPath)...) //nolint:gosec // executable is resolved from PATH or explicit admin env.
	hideCommandWindow(cmd)
	cmd.Stdout = processLogWriter{logger: s.logger, botID: botID}
	cmd.Stderr = processLogWriter{logger: s.logger, botID: botID}
	if err := cmd.Run(); err != nil {
		return nil, "", fmt.Errorf("capture display screenshot: %w", err)
	}
	data, err := os.ReadFile(outputPath) //nolint:gosec // outputPath is a freshly-created temp file.
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 {
		return nil, "", errors.New("display screenshot is empty")
	}
	data, err = limitJPEGSize(data, screenshotMaxBytes)
	if err != nil {
		return nil, "", err
	}
	return data, screenshotMIME, nil
}

func (s *Service) ControlInput(ctx context.Context, botID string, event ControlInput) error {
	return s.ControlInputs(ctx, botID, []ControlInput{event})
}

func (s *Service) ControlInputs(ctx context.Context, botID string, events []ControlInput) error {
	if s == nil || s.workspace == nil {
		return ErrManagerUnavailable
	}
	if !s.workspace.BotDisplayEnabled(ctx, botID) {
		return ErrDisplayDisabled
	}
	if !s.displayReachable(ctx, botID) {
		return fmt.Errorf("%w: %s", ErrDisplayUnavailable, s.displayTarget(botID))
	}
	conn, err := s.dialRFB(ctx, botID)
	if err != nil {
		return fmt.Errorf("connect display input: %w", err)
	}
	input, err := newRFBInputClient(conn)
	if err != nil {
		return fmt.Errorf("connect display input: %w", err)
	}
	defer func() { _ = input.Close() }()
	for _, event := range events {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch event.Type {
		case "pointer":
			if err := input.Pointer(event.X, event.Y, event.ButtonMask); err != nil {
				return err
			}
		case "key":
			if event.Keysym == 0 {
				return errors.New("keysym is required")
			}
			if err := input.Key(event.Keysym, event.Down); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported input event type %q", event.Type)
		}
	}
	return nil
}

func (s *Service) session(ctx context.Context, botID, gstLaunch, codec string) (*session, error) {
	s.mu.Lock()
	if sess := s.sessions[botID]; sess != nil && !sess.closed() {
		s.mu.Unlock()
		// Display sessions are shared across viewers via RTP fan-out. If a new
		// viewer needs a different codec, we refuse rather than tearing down
		// the existing pipeline — that would black out anyone already watching.
		if sess.codec != codec {
			return nil, fmt.Errorf("%w: another viewer is already using %s", ErrCodecUnsupported, sess.codec)
		}
		return sess, nil
	}
	if start := s.starting[botID]; start != nil {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-start.done:
		}
		if start.err != nil {
			return nil, start.err
		}
		if start.sess == nil || start.sess.closed() {
			return nil, fmt.Errorf("%w: display pipeline is not running", ErrEncoderUnavailable)
		}
		if start.sess.codec != codec {
			return nil, fmt.Errorf("%w: another viewer is already using %s", ErrCodecUnsupported, start.sess.codec)
		}
		return start.sess, nil
	}
	start := &sessionStart{done: make(chan struct{})}
	s.starting[botID] = start
	s.mu.Unlock()

	sess := newSession(s, botID, gstLaunch, codec)
	if err := sess.start(ctx); err != nil {
		sess.stop()
		s.finishSessionStart(botID, start, nil, err)
		return nil, err
	}

	s.mu.Lock()
	current := s.sessions[botID]
	if current == nil || current.closed() {
		s.sessions[botID] = sess
		s.mu.Unlock()
		s.finishSessionStart(botID, start, sess, nil)
		return sess, nil
	}
	s.mu.Unlock()

	sess.stop()
	if current.codec != codec {
		err := fmt.Errorf("%w: another viewer is already using %s", ErrCodecUnsupported, current.codec)
		s.finishSessionStart(botID, start, nil, err)
		return nil, err
	}
	s.finishSessionStart(botID, start, current, nil)
	return current, nil
}

func (s *Service) finishSessionStart(botID string, start *sessionStart, sess *session, err error) {
	start.sess = sess
	start.err = err
	s.mu.Lock()
	if s.starting[botID] == start {
		delete(s.starting, botID)
	}
	s.mu.Unlock()
	close(start.done)
}

func (s *Service) removeSession(botID string, target *session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current := s.sessions[botID]; current == target {
		delete(s.sessions, botID)
	}
}

type session struct {
	service   *Service
	botID     string
	gstLaunch string
	codec     string

	ctx          context.Context
	cancel       context.CancelFunc
	runCtxCancel context.CancelFunc

	proxy net.Listener
	udp   *net.UDPConn
	cmd   *exec.Cmd

	tracksMu sync.RWMutex
	tracks   map[string]*webrtc.TrackLocalStaticRTP
	input    *rfbInputClient

	cursorMu           sync.RWMutex
	cursorPayload      []byte
	cursorDataChannels map[string]*webrtc.DataChannel

	peersMu sync.RWMutex
	peers   map[string]*peerSession

	stopOnce sync.Once
}

type peerSession struct {
	id        string
	codec     string
	createdAt time.Time
	trackID   string

	mu    sync.RWMutex
	state string

	closeOnce sync.Once
	close     func()
}

func newSession(service *Service, botID, gstLaunch, codec string) *session {
	ctx, cancel := context.WithCancel(context.Background()) //nolint:gosec // cancel is stored on the session and called when the display session stops.
	return &session{
		service:            service,
		botID:              botID,
		gstLaunch:          gstLaunch,
		codec:              codec,
		ctx:                ctx,
		cancel:             cancel,
		tracks:             make(map[string]*webrtc.TrackLocalStaticRTP),
		peers:              make(map[string]*peerSession),
		cursorDataChannels: make(map[string]*webrtc.DataChannel),
	}
}

func (s *session) closed() bool {
	select {
	case <-s.ctx.Done():
		return true
	default:
		return false
	}
}

func (s *session) start(ctx context.Context) error {
	if !s.service.displayReachable(ctx, s.botID) {
		return fmt.Errorf("%w: %s", ErrDisplayUnavailable, s.service.displayTarget(s.botID))
	}

	runCtx, runCtxCancel := context.WithCancel(context.WithoutCancel(ctx))
	cancelRunCtx := true
	defer func() {
		if cancelRunCtx {
			runCtxCancel()
		}
	}()
	go func() {
		<-s.ctx.Done()
		runCtxCancel()
	}()

	listenConfig := net.ListenConfig{}
	proxy, err := listenConfig.Listen(runCtx, "tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("start RFB tcp shim: %w", err)
	}
	s.proxy = proxy

	udpAddr, err := net.ResolveUDPAddr("udp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("resolve RTP udp address: %w", err)
	}
	udp, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		return fmt.Errorf("listen RTP udp: %w", err)
	}
	s.udp = udp

	proxyPort := proxy.Addr().(*net.TCPAddr).Port
	rtpPort := udp.LocalAddr().(*net.UDPAddr).Port
	args := gstreamerArgs(s.codec, proxyPort, rtpPort)
	cmd := exec.CommandContext(runCtx, s.gstLaunch, args...) //nolint:gosec // executable is resolved from PATH or explicit admin env.
	hideCommandWindow(cmd)
	cmd.Stdout = processLogWriter{logger: s.service.logger, botID: s.botID}
	cmd.Stderr = processLogWriter{logger: s.service.logger, botID: s.botID}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start gstreamer display pipeline: %w", err)
	}
	s.cmd = cmd

	if conn, err := s.service.dialRFB(runCtx, s.botID); err == nil {
		input, inputErr := newRFBInputClient(conn)
		if inputErr != nil {
			_ = conn.Close()
			s.service.logger.Warn("display input channel unavailable", slog.String("bot_id", s.botID), slog.Any("error", inputErr))
		} else {
			s.input = input
		}
	} else {
		s.service.logger.Warn("display input channel unavailable", slog.String("bot_id", s.botID), slog.Any("error", err))
	}
	s.runCtxCancel = runCtxCancel
	cancelRunCtx = false

	s.service.logger.Info("display encoder started",
		slog.String("bot_id", s.botID),
		slog.String("rfb_target", s.service.displayTarget(s.botID)),
		slog.String("gst_launch", s.gstLaunch),
		slog.String("codec", s.codec),
		slog.Int("proxy_port", proxyPort),
		slog.Int("rtp_port", rtpPort),
		slog.Int("pid", cmd.Process.Pid),
	)

	go s.acceptProxy()
	go s.forwardRTP()
	go s.watchCursor(runCtx)
	gstreamerDone := make(chan error, 1)
	go s.waitGStreamer(gstreamerDone)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-gstreamerDone:
		if err != nil {
			return fmt.Errorf("%w: display pipeline exited during startup: %w", ErrEncoderUnavailable, err)
		}
		return fmt.Errorf("%w: display pipeline exited during startup", ErrEncoderUnavailable)
	case <-time.After(150 * time.Millisecond):
		if s.closed() {
			return fmt.Errorf("%w: display pipeline exited during startup", ErrEncoderUnavailable)
		}
		return nil
	}
}

func (s *session) answer(ctx context.Context, req OfferRequest) (OfferResponse, error) {
	if s.closed() {
		return OfferResponse{}, fmt.Errorf("%w: display pipeline is not running", ErrEncoderUnavailable)
	}
	sessionID := strings.TrimSpace(req.SessionID)
	if sessionID == "" {
		sessionID = uuid.NewString()
	}
	s.closeStalePeers(time.Now())
	previousPeer := s.peer(sessionID)

	mediaEngine := &webrtc.MediaEngine{}
	if err := registerVideoCodec(mediaEngine, s.codec); err != nil {
		return OfferResponse{}, err
	}

	api, rtcCfg, err := newWebRTCAPI(mediaEngine, req.NATIPs)
	if err != nil {
		return OfferResponse{}, err
	}
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return OfferResponse{}, err
	}
	if rtcCfg.UDPPortMin != 0 || len(rtcCfg.NATIPs) > 0 {
		s.service.logger.Info("display webrtc configured",
			slog.String("bot_id", s.botID),
			slog.Int("udp_port_min", int(rtcCfg.UDPPortMin)),
			slog.Int("udp_port_max", int(rtcCfg.UDPPortMax)),
			slog.Any("nat_ips", rtcCfg.NATIPs),
		)
	}

	track, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{
		MimeType:  s.codec,
		ClockRate: videoClockRate,
	}, "video", "display-"+s.botID)
	if err != nil {
		_ = pc.Close()
		return OfferResponse{}, err
	}

	sender, err := pc.AddTrack(track)
	if err != nil {
		_ = pc.Close()
		return OfferResponse{}, err
	}
	go drainRTCP(sender)

	cursorChannel, err := pc.CreateDataChannel(cursorChannelLabel, nil)
	if err != nil {
		_ = pc.Close()
		return OfferResponse{}, fmt.Errorf("create cursor metadata channel: %w", err)
	}
	cursorChannelID := uuid.NewString()
	cursorChannel.OnOpen(func() {
		s.sendLatestCursor(cursorChannel)
	})
	s.addCursorDataChannel(cursorChannelID, cursorChannel)

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() != inputChannelLabel {
			return
		}
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			if err := s.handleInput(msg.Data); err != nil {
				s.service.logger.Debug("display input event dropped", slog.String("bot_id", s.botID), slog.Any("error", err))
			}
		})
	})

	trackID := uuid.NewString()
	s.addTrack(trackID, track)
	peer := &peerSession{
		id:        sessionID,
		codec:     s.codec,
		createdAt: time.Now(),
		state:     "new",
		trackID:   trackID,
	}

	var cleanupOnce sync.Once
	cleanup := func(closePeer bool) {
		cleanupOnce.Do(func() {
			s.removePeer(peer)
			s.removeTrack(trackID)
			s.removeCursorDataChannel(cursorChannelID)
			if closePeer {
				_ = pc.Close()
			}
		})
	}
	peer.close = func() { cleanup(true) }
	s.addPeer(peer)
	if previousPeer != nil {
		previousPeer.closeNow()
	}

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		peer.setState(state.String())
		s.service.logger.Info("display webrtc connection state",
			slog.String("bot_id", s.botID),
			slog.String("session_id", sessionID),
			slog.String("state", state.String()),
		)
		switch state {
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateDisconnected:
			cleanup(true)
		case webrtc.PeerConnectionStateClosed:
			cleanup(false)
		default:
		}
	})
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		s.service.logger.Info("display webrtc ice state",
			slog.String("bot_id", s.botID),
			slog.String("session_id", sessionID),
			slog.String("state", state.String()),
		)
	})

	if err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  req.SDP,
	}); err != nil {
		cleanup(true)
		return OfferResponse{}, err
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		cleanup(true)
		return OfferResponse{}, err
	}
	gatherDone := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		cleanup(true)
		return OfferResponse{}, err
	}

	select {
	case <-ctx.Done():
		cleanup(true)
		return OfferResponse{}, ctx.Err()
	case <-gatherDone:
	}

	local := pc.LocalDescription()
	if local == nil {
		cleanup(true)
		return OfferResponse{}, errors.New("local session description unavailable")
	}

	return OfferResponse{Type: "answer", SDP: local.SDP, SessionID: sessionID}, nil
}

type inputEvent struct {
	Type       string `json:"type"`
	X          int    `json:"x,omitempty"`
	Y          int    `json:"y,omitempty"`
	ButtonMask uint8  `json:"button_mask,omitempty"`
	Keysym     uint32 `json:"keysym,omitempty"`
	Down       bool   `json:"down,omitempty"`
}

type cursorMetadata struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	Cursor string `json:"cursor,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	HotX   int    `json:"hot_x,omitempty"`
	HotY   int    `json:"hot_y,omitempty"`
	Hidden bool   `json:"hidden,omitempty"`
}

func (s *session) handleInput(data []byte) error {
	if s.input == nil {
		return errors.New("display input is unavailable")
	}
	var event inputEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return err
	}
	switch event.Type {
	case "pointer":
		return s.input.Pointer(event.X, event.Y, event.ButtonMask)
	case "key":
		if event.Keysym == 0 {
			return errors.New("keysym is required")
		}
		return s.input.Key(event.Keysym, event.Down)
	default:
		return fmt.Errorf("unsupported input event type %q", event.Type)
	}
}

func (s *session) addTrack(id string, track *webrtc.TrackLocalStaticRTP) {
	s.tracksMu.Lock()
	s.tracks[id] = track
	s.tracksMu.Unlock()
}

func (s *session) removeTrack(id string) {
	s.tracksMu.Lock()
	delete(s.tracks, id)
	empty := len(s.tracks) == 0
	s.tracksMu.Unlock()
	if empty {
		go s.stop()
	}
}

func (s *session) addCursorDataChannel(id string, channel *webrtc.DataChannel) {
	if channel == nil || strings.TrimSpace(id) == "" {
		return
	}
	s.cursorMu.Lock()
	s.cursorDataChannels[id] = channel
	s.cursorMu.Unlock()
}

func (s *session) removeCursorDataChannel(id string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	s.cursorMu.Lock()
	delete(s.cursorDataChannels, id)
	s.cursorMu.Unlock()
}

func (s *session) sendLatestCursor(channel *webrtc.DataChannel) {
	if channel == nil {
		return
	}
	s.cursorMu.RLock()
	payload := append([]byte(nil), s.cursorPayload...)
	s.cursorMu.RUnlock()
	if len(payload) == 0 {
		return
	}
	s.sendCursorPayload(channel, payload)
}

func (s *session) broadcastCursor(cursor cursorMetadata) {
	payload, err := json.Marshal(cursor)
	if err != nil {
		s.service.logger.Debug("display cursor metadata dropped", slog.String("bot_id", s.botID), slog.Any("error", err))
		return
	}
	s.cursorMu.Lock()
	if bytes.Equal(s.cursorPayload, payload) {
		s.cursorMu.Unlock()
		return
	}
	s.cursorPayload = append(s.cursorPayload[:0], payload...)
	channels := make([]*webrtc.DataChannel, 0, len(s.cursorDataChannels))
	for _, channel := range s.cursorDataChannels {
		channels = append(channels, channel)
	}
	s.cursorMu.Unlock()

	for _, channel := range channels {
		s.sendCursorPayload(channel, payload)
	}
}

func (s *session) sendCursorPayload(channel *webrtc.DataChannel, payload []byte) {
	if channel == nil || channel.ReadyState() != webrtc.DataChannelStateOpen {
		return
	}
	if err := channel.SendText(string(payload)); err != nil {
		s.service.logger.Debug("display cursor metadata send failed", slog.String("bot_id", s.botID), slog.Any("error", err))
	}
}

func (s *session) watchCursor(ctx context.Context) {
	backoff := 500 * time.Millisecond
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		conn, err := s.service.dialRFB(ctx, s.botID)
		if err != nil {
			s.service.logger.Debug("display cursor watcher unavailable", slog.String("bot_id", s.botID), slog.Any("error", err))
			if waitContext(ctx, backoff) {
				return
			}
			if backoff < 5*time.Second {
				backoff *= 2
			}
			continue
		}
		watcher := &rfbCursorWatcher{
			conn: conn,
			onCursor: func(cursor cursorMetadata) {
				s.broadcastCursor(cursor)
			},
		}
		err = watcher.Run(ctx)
		if err := ctx.Err(); err != nil {
			return
		}
		if err != nil {
			s.service.logger.Debug("display cursor watcher stopped", slog.String("bot_id", s.botID), slog.Any("error", err))
		}
		if waitContext(ctx, backoff) {
			return
		}
		if backoff < 5*time.Second {
			backoff *= 2
		}
	}
}

func waitContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return true
	case <-timer.C:
		return false
	}
}

func (s *session) addPeer(peer *peerSession) {
	s.peersMu.Lock()
	s.peers[peer.id] = peer
	s.peersMu.Unlock()
}

func (s *session) removePeer(peer *peerSession) {
	if peer == nil {
		return
	}
	s.peersMu.Lock()
	if current := s.peers[peer.id]; current == peer {
		delete(s.peers, peer.id)
	}
	s.peersMu.Unlock()
}

func (s *session) peer(id string) *peerSession {
	s.peersMu.RLock()
	defer s.peersMu.RUnlock()
	return s.peers[strings.TrimSpace(id)]
}

func (s *session) peerInfos() []SessionInfo {
	s.peersMu.RLock()
	defer s.peersMu.RUnlock()
	infos := make([]SessionInfo, 0, len(s.peers))
	for _, peer := range s.peers {
		infos = append(infos, peer.info())
	}
	return infos
}

func (s *session) closePeer(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	s.peersMu.RLock()
	peer := s.peers[id]
	s.peersMu.RUnlock()
	if peer == nil {
		return false
	}
	peer.closeNow()
	return true
}

func (s *session) closeStalePeers(now time.Time) {
	s.peersMu.RLock()
	stale := make([]*peerSession, 0)
	for _, peer := range s.peers {
		if peer.stale(now) {
			stale = append(stale, peer)
		}
	}
	s.peersMu.RUnlock()
	for _, peer := range stale {
		peer.closeNow()
	}
}

func (p *peerSession) setState(state string) {
	p.mu.Lock()
	p.state = state
	p.mu.Unlock()
}

func (p *peerSession) info() SessionInfo {
	p.mu.RLock()
	state := p.state
	p.mu.RUnlock()
	return SessionInfo{
		ID:        p.id,
		Codec:     p.codec,
		State:     state,
		CreatedAt: p.createdAt,
	}
}

func (p *peerSession) closeNow() {
	p.closeOnce.Do(func() {
		if p.close != nil {
			p.close()
		}
	})
}

func (p *peerSession) stale(now time.Time) bool {
	p.mu.RLock()
	state := p.state
	p.mu.RUnlock()
	switch state {
	case webrtc.PeerConnectionStateClosed.String(),
		webrtc.PeerConnectionStateDisconnected.String(),
		webrtc.PeerConnectionStateFailed.String():
		return true
	case webrtc.PeerConnectionStateNew.String(),
		webrtc.PeerConnectionStateConnecting.String():
		return now.Sub(p.createdAt) > stalePeerTTL
	default:
		return false
	}
}

func (s *session) stop() {
	s.stopOnce.Do(func() {
		s.cancel()
		if s.runCtxCancel != nil {
			s.runCtxCancel()
		}
		if s.proxy != nil {
			_ = s.proxy.Close()
		}
		if s.udp != nil {
			_ = s.udp.Close()
		}
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		if s.input != nil {
			_ = s.input.Close()
		}
		s.service.removeSession(s.botID, s)
		s.service.logger.Info("display encoder stopped", slog.String("bot_id", s.botID))
	})
}

func (s *session) acceptProxy() {
	for {
		conn, err := s.proxy.Accept()
		if err != nil {
			if s.ctx.Err() == nil {
				s.service.logger.Warn("display RFB shim stopped", slog.String("bot_id", s.botID), slog.Any("error", err))
			}
			return
		}
		go s.proxyRFB(conn)
	}
}

func (s *session) proxyRFB(conn net.Conn) {
	proxyRFBConnection(s.ctx, conn, func(ctx context.Context) (net.Conn, error) {
		return s.service.dialRFB(ctx, s.botID)
	}, s.service.logger, s.botID, false)
}

func proxyRFBListener(ctx context.Context, listener net.Listener, dialRFB func(context.Context) (net.Conn, error), logger *slog.Logger, botID string, filterCursor bool) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() == nil && !errors.Is(err, net.ErrClosed) {
				logger.Warn("display RFB screenshot shim stopped", slog.String("bot_id", botID), slog.Any("error", err))
			}
			return
		}
		go proxyRFBConnection(ctx, conn, dialRFB, logger, botID, filterCursor)
	}
}

func proxyRFBConnection(ctx context.Context, conn net.Conn, dialRFB func(context.Context) (net.Conn, error), logger *slog.Logger, botID string, filterCursor bool) {
	defer func() { _ = conn.Close() }()

	rfbConn, err := dialRFB(ctx)
	if err != nil {
		logger.Warn("display RFB dial failed", slog.String("bot_id", botID), slog.Any("error", err))
		return
	}
	defer func() { _ = rfbConn.Close() }()

	if filterCursor {
		if err := proxyRFBWithoutCursor(ctx, conn, rfbConn); err != nil && ctx.Err() == nil && !errors.Is(err, net.ErrClosed) {
			logger.Debug("display RFB cursor-filter proxy stopped", slog.String("bot_id", botID), slog.Any("error", err))
		}
		return
	}

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(conn, rfbConn)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(rfbConn, conn)
		done <- struct{}{}
	}()
	<-done
}

func proxyRFBWithoutCursor(ctx context.Context, client, server net.Conn) error {
	format, err := proxyRFBHandshake(client, server)
	if err != nil {
		return err
	}

	errCh := make(chan error, 2)
	go func() {
		errCh <- proxyRFBClientMessages(ctx, client, server, &format)
	}()
	go func() {
		errCh <- proxyRFBServerMessages(ctx, client, server, &format)
	}()
	return <-errCh
}

func proxyRFBHandshake(client, server net.Conn) (rfbPixelFormat, error) {
	version := make([]byte, 12)
	if _, err := io.ReadFull(server, version); err != nil {
		return rfbPixelFormat{}, fmt.Errorf("proxy read RFB version: %w", err)
	}
	if _, err := client.Write(version); err != nil {
		return rfbPixelFormat{}, fmt.Errorf("proxy write RFB version: %w", err)
	}
	if _, err := io.ReadFull(client, version); err != nil {
		return rfbPixelFormat{}, fmt.Errorf("proxy read RFB client version: %w", err)
	}
	if _, err := server.Write(version); err != nil {
		return rfbPixelFormat{}, fmt.Errorf("proxy write RFB client version: %w", err)
	}

	securityCount := []byte{0}
	if _, err := io.ReadFull(server, securityCount); err != nil {
		return rfbPixelFormat{}, fmt.Errorf("proxy read RFB security count: %w", err)
	}
	if _, err := client.Write(securityCount); err != nil {
		return rfbPixelFormat{}, fmt.Errorf("proxy write RFB security count: %w", err)
	}
	if securityCount[0] == 0 {
		if err := proxyRFBLengthPrefixedBytes(server, client); err != nil {
			return rfbPixelFormat{}, err
		}
		return rfbPixelFormat{}, errors.New("RFB security negotiation failed")
	}
	securityTypes := make([]byte, int(securityCount[0]))
	if _, err := io.ReadFull(server, securityTypes); err != nil {
		return rfbPixelFormat{}, fmt.Errorf("proxy read RFB security types: %w", err)
	}
	if _, err := client.Write(securityTypes); err != nil {
		return rfbPixelFormat{}, fmt.Errorf("proxy write RFB security types: %w", err)
	}
	securityType := []byte{0}
	if _, err := io.ReadFull(client, securityType); err != nil {
		return rfbPixelFormat{}, fmt.Errorf("proxy read RFB selected security type: %w", err)
	}
	if _, err := server.Write(securityType); err != nil {
		return rfbPixelFormat{}, fmt.Errorf("proxy write RFB selected security type: %w", err)
	}
	securityResult := make([]byte, 4)
	if _, err := io.ReadFull(server, securityResult); err != nil {
		return rfbPixelFormat{}, fmt.Errorf("proxy read RFB security result: %w", err)
	}
	if _, err := client.Write(securityResult); err != nil {
		return rfbPixelFormat{}, fmt.Errorf("proxy write RFB security result: %w", err)
	}
	if binary.BigEndian.Uint32(securityResult) != 0 {
		if err := proxyRFBLengthPrefixedBytes(server, client); err != nil {
			return rfbPixelFormat{}, err
		}
		return rfbPixelFormat{}, errors.New("RFB security rejected")
	}

	clientInit := []byte{0}
	if _, err := io.ReadFull(client, clientInit); err != nil {
		return rfbPixelFormat{}, fmt.Errorf("proxy read RFB client init: %w", err)
	}
	if _, err := server.Write(clientInit); err != nil {
		return rfbPixelFormat{}, fmt.Errorf("proxy write RFB client init: %w", err)
	}
	serverInit := make([]byte, 24)
	if _, err := io.ReadFull(server, serverInit); err != nil {
		return rfbPixelFormat{}, fmt.Errorf("proxy read RFB server init: %w", err)
	}
	if _, err := client.Write(serverInit); err != nil {
		return rfbPixelFormat{}, fmt.Errorf("proxy write RFB server init: %w", err)
	}
	nameLen := binary.BigEndian.Uint32(serverInit[20:24])
	if nameLen > maxRFBDiscardBytes {
		return rfbPixelFormat{}, fmt.Errorf("RFB desktop name too large: %d", nameLen)
	}
	if nameLen > 0 {
		name := make([]byte, nameLen)
		if _, err := io.ReadFull(server, name); err != nil {
			return rfbPixelFormat{}, fmt.Errorf("proxy read RFB desktop name: %w", err)
		}
		if _, err := client.Write(name); err != nil {
			return rfbPixelFormat{}, fmt.Errorf("proxy write RFB desktop name: %w", err)
		}
	}
	return parseRFBPixelFormat(serverInit[4:20]), nil
}

func proxyRFBLengthPrefixedBytes(src, dst net.Conn) error {
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(src, lengthBuf); err != nil {
		return fmt.Errorf("proxy read RFB length: %w", err)
	}
	if _, err := dst.Write(lengthBuf); err != nil {
		return fmt.Errorf("proxy write RFB length: %w", err)
	}
	length := binary.BigEndian.Uint32(lengthBuf)
	if length > maxRFBDiscardBytes {
		return fmt.Errorf("RFB length-prefixed payload too large: %d", length)
	}
	if _, err := io.CopyN(dst, src, int64(length)); err != nil {
		return fmt.Errorf("proxy copy RFB length-prefixed payload: %w", err)
	}
	return nil
}

func proxyRFBClientMessages(ctx context.Context, client, server net.Conn, format *rfbPixelFormat) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		msgType := []byte{0}
		if _, err := io.ReadFull(client, msgType); err != nil {
			return fmt.Errorf("proxy read RFB client message type: %w", err)
		}
		switch msgType[0] {
		case 0:
			payload := make([]byte, 19)
			if _, err := io.ReadFull(client, payload); err != nil {
				return fmt.Errorf("proxy read RFB set pixel format: %w", err)
			}
			*format = parseRFBPixelFormat(payload[3:19])
			if _, err := server.Write(append(msgType, payload...)); err != nil {
				return fmt.Errorf("proxy write RFB set pixel format: %w", err)
			}
		case 2:
			if err := proxyRFBSetEncodingsWithCursor(client, server); err != nil {
				return err
			}
		case 3:
			if err := proxyFixedRFBClientMessage(client, server, msgType, 9, "framebuffer update request"); err != nil {
				return err
			}
		case 4:
			if err := proxyFixedRFBClientMessage(client, server, msgType, 7, "key event"); err != nil {
				return err
			}
		case 5:
			if err := proxyFixedRFBClientMessage(client, server, msgType, 5, "pointer event"); err != nil {
				return err
			}
		case 6:
			if err := proxyRFBClientCutText(client, server, msgType); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported RFB client message type %d", msgType[0])
		}
	}
}

func proxyFixedRFBClientMessage(client io.Reader, server io.Writer, msgType []byte, payloadSize int, name string) error {
	payload := make([]byte, payloadSize)
	if _, err := io.ReadFull(client, payload); err != nil {
		return fmt.Errorf("proxy read RFB %s: %w", name, err)
	}
	if _, err := server.Write(append(msgType, payload...)); err != nil {
		return fmt.Errorf("proxy write RFB %s: %w", name, err)
	}
	return nil
}

func proxyRFBClientCutText(client io.Reader, server io.Writer, msgType []byte) error {
	header := make([]byte, 7)
	if _, err := io.ReadFull(client, header); err != nil {
		return fmt.Errorf("proxy read RFB client cut text header: %w", err)
	}
	length := binary.BigEndian.Uint32(header[3:7])
	if length > maxRFBDiscardBytes {
		return fmt.Errorf("RFB client cut text too large: %d", length)
	}
	if _, err := server.Write(append(msgType, header...)); err != nil {
		return fmt.Errorf("proxy write RFB client cut text header: %w", err)
	}
	if _, err := io.CopyN(server, client, int64(length)); err != nil {
		return fmt.Errorf("proxy copy RFB client cut text: %w", err)
	}
	return nil
}

func proxyRFBSetEncodingsWithCursor(client io.Reader, server io.Writer) error {
	header := make([]byte, 3)
	if _, err := io.ReadFull(client, header); err != nil {
		return fmt.Errorf("proxy read RFB set encodings header: %w", err)
	}
	count := int(binary.BigEndian.Uint16(header[1:3]))
	buf := make([]byte, count*4)
	if _, err := io.ReadFull(client, buf); err != nil {
		return fmt.Errorf("proxy read RFB set encodings payload: %w", err)
	}
	seen := map[int32]bool{}
	encodings := []int32{rfbEncodingCursor, rfbEncodingXCursor}
	seen[rfbEncodingCursor] = true
	seen[rfbEncodingXCursor] = true
	for i := 0; i < count; i++ {
		encoding := int32(binary.BigEndian.Uint32(buf[i*4 : i*4+4])) //nolint:gosec // RFB encodings are signed 32-bit values on the wire.
		if encoding != rfbEncodingRaw && encoding != rfbEncodingCopyRect {
			continue
		}
		if seen[encoding] {
			continue
		}
		encodings = append(encodings, encoding)
		seen[encoding] = true
	}
	if !seen[rfbEncodingRaw] {
		encodings = append(encodings, rfbEncodingRaw)
	}
	return writeRFBSetEncodings(server, encodings)
}

func proxyRFBServerMessages(ctx context.Context, client, server net.Conn, format *rfbPixelFormat) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		msgType := []byte{0}
		if _, err := io.ReadFull(server, msgType); err != nil {
			return fmt.Errorf("proxy read RFB server message type: %w", err)
		}
		switch msgType[0] {
		case 0:
			if err := proxyRFBFramebufferUpdateWithoutCursor(client, server, *format); err != nil {
				return err
			}
		case 2:
			if _, err := client.Write(msgType); err != nil {
				return fmt.Errorf("proxy write RFB bell: %w", err)
			}
		case 3:
			if err := proxyRFBServerCutText(client, server, msgType); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported RFB server message type %d", msgType[0])
		}
	}
}

func proxyRFBServerCutText(client io.Writer, server io.Reader, msgType []byte) error {
	header := make([]byte, 7)
	if _, err := io.ReadFull(server, header); err != nil {
		return fmt.Errorf("proxy read RFB server cut text header: %w", err)
	}
	length := binary.BigEndian.Uint32(header[3:7])
	if length > maxRFBDiscardBytes {
		return fmt.Errorf("RFB server cut text too large: %d", length)
	}
	if _, err := client.Write(append(msgType, header...)); err != nil {
		return fmt.Errorf("proxy write RFB server cut text header: %w", err)
	}
	if _, err := io.CopyN(client, server, int64(length)); err != nil {
		return fmt.Errorf("proxy copy RFB server cut text: %w", err)
	}
	return nil
}

func proxyRFBFramebufferUpdateWithoutCursor(client io.Writer, server io.Reader, format rfbPixelFormat) error {
	header := make([]byte, 3)
	if _, err := io.ReadFull(server, header); err != nil {
		return fmt.Errorf("proxy read RFB framebuffer update header: %w", err)
	}
	count := int(binary.BigEndian.Uint16(header[1:3]))
	var payload bytes.Buffer
	outCount := 0
	for i := 0; i < count; i++ {
		rect, rectHeader, err := readRFBRectHeaderBytes(server)
		if err != nil {
			return err
		}
		switch rect.encoding {
		case rfbEncodingCursor:
			if err := discardRFBCursor(server, rect, format); err != nil {
				return err
			}
		case rfbEncodingXCursor:
			if err := discardRFBXCursor(server, rect); err != nil {
				return err
			}
		case rfbEncodingRaw:
			payload.Write(rectHeader)
			if err := copyRFBRectangle(&payload, server, rect, format.bytesPerPixel()); err != nil {
				return err
			}
			outCount++
		case rfbEncodingCopyRect:
			payload.Write(rectHeader)
			if _, err := io.CopyN(&payload, server, 4); err != nil {
				return fmt.Errorf("proxy copy RFB copyrect payload: %w", err)
			}
			outCount++
		default:
			return fmt.Errorf("unsupported RFB rectangle encoding %d", rect.encoding)
		}
	}
	outHeader := []byte{0, 0, 0, 0}
	binary.BigEndian.PutUint16(outHeader[2:4], uint16(outCount))
	if _, err := client.Write(outHeader); err != nil {
		return fmt.Errorf("proxy write RFB framebuffer update header: %w", err)
	}
	if _, err := client.Write(payload.Bytes()); err != nil {
		return fmt.Errorf("proxy write RFB framebuffer update payload: %w", err)
	}
	return nil
}

func readRFBRectHeaderBytes(r io.Reader) (rfbRectHeader, []byte, error) {
	buf := make([]byte, 12)
	if _, err := io.ReadFull(r, buf); err != nil {
		return rfbRectHeader{}, nil, fmt.Errorf("read RFB rectangle header: %w", err)
	}
	return parseRFBRectHeader(buf), buf, nil
}

func copyRFBRectangle(dst io.Writer, src io.Reader, rect rfbRectHeader, bytesPerPixel int) error {
	if bytesPerPixel <= 0 {
		return errors.New("invalid RFB pixel format")
	}
	size := int64(rect.width) * int64(rect.height) * int64(bytesPerPixel)
	if size < 0 || size > maxRFBDiscardBytes {
		return fmt.Errorf("RFB rectangle payload too large: %d", size)
	}
	if _, err := io.CopyN(dst, src, size); err != nil {
		return fmt.Errorf("proxy copy RFB raw rectangle: %w", err)
	}
	return nil
}

func discardRFBCursor(r io.Reader, rect rfbRectHeader, format rfbPixelFormat) error {
	bytesPerPixel := format.bytesPerPixel()
	if bytesPerPixel <= 0 {
		return errors.New("invalid RFB cursor pixel format")
	}
	size := int64(rect.width)*int64(rect.height)*int64(bytesPerPixel) + int64(rfbCursorMaskBytes(rect.width, rect.height))
	if size < 0 || size > maxRFBDiscardBytes {
		return fmt.Errorf("RFB cursor payload too large: %d", size)
	}
	if _, err := io.CopyN(io.Discard, r, size); err != nil {
		return fmt.Errorf("discard RFB cursor payload: %w", err)
	}
	return nil
}

func discardRFBXCursor(r io.Reader, rect rfbRectHeader) error {
	size := int64(6 + rfbCursorMaskBytes(rect.width, rect.height)*2)
	if size < 0 || size > maxRFBDiscardBytes {
		return fmt.Errorf("RFB XCursor payload too large: %d", size)
	}
	if _, err := io.CopyN(io.Discard, r, size); err != nil {
		return fmt.Errorf("discard RFB XCursor payload: %w", err)
	}
	return nil
}

func (s *session) forwardRTP() {
	buf := make([]byte, 4096)
	for {
		n, _, err := s.udp.ReadFromUDP(buf)
		if err != nil {
			if s.ctx.Err() == nil {
				s.service.logger.Warn("display RTP reader stopped", slog.String("bot_id", s.botID), slog.Any("error", err))
				s.stop()
			}
			return
		}
		var pkt rtp.Packet
		if err := pkt.Unmarshal(buf[:n]); err != nil {
			s.service.logger.Debug("display RTP packet dropped", slog.String("bot_id", s.botID), slog.Any("error", err))
			continue
		}

		s.tracksMu.RLock()
		for _, track := range s.tracks {
			pktCopy := pkt
			if err := track.WriteRTP(&pktCopy); err != nil {
				s.service.logger.Debug("display RTP write failed", slog.String("bot_id", s.botID), slog.Any("error", err))
			}
		}
		s.tracksMu.RUnlock()
	}
}

func (s *session) waitGStreamer(done chan<- error) {
	err := s.cmd.Wait()
	if done != nil {
		select {
		case done <- err:
		default:
		}
	}
	if s.ctx.Err() == nil {
		s.service.logger.Warn("display gstreamer pipeline exited", slog.String("bot_id", s.botID), slog.Any("error", err))
		s.stop()
	}
}

func drainRTCP(sender *webrtc.RTPSender) {
	buf := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buf); err != nil {
			return
		}
	}
}

func newWebRTCAPI(mediaEngine *webrtc.MediaEngine, inferredNATIPs []string) (*webrtc.API, rtcSettings, error) {
	cfg, err := readRTCSettings(inferredNATIPs)
	if err != nil {
		return nil, rtcSettings{}, err
	}

	settingEngine := webrtc.SettingEngine{}
	if cfg.UDPPortMin != 0 || cfg.UDPPortMax != 0 {
		if err := settingEngine.SetEphemeralUDPPortRange(cfg.UDPPortMin, cfg.UDPPortMax); err != nil {
			return nil, rtcSettings{}, fmt.Errorf("configure display WebRTC UDP port range: %w", err)
		}
	}
	if len(cfg.NATIPs) > 0 {
		if err := settingEngine.SetICEAddressRewriteRules(webrtc.ICEAddressRewriteRule{
			External:        cfg.NATIPs,
			AsCandidateType: webrtc.ICECandidateTypeHost,
			Mode:            webrtc.ICEAddressRewriteReplace,
		}); err != nil {
			return nil, rtcSettings{}, fmt.Errorf("configure display WebRTC NAT rewrite: %w", err)
		}
	}

	return webrtc.NewAPI(webrtc.WithMediaEngine(mediaEngine), webrtc.WithSettingEngine(settingEngine)), cfg, nil
}

func readRTCSettings(inferredNATIPs []string) (rtcSettings, error) {
	var cfg rtcSettings
	minRaw := strings.TrimSpace(os.Getenv(rtcUDPPortMinEnv))
	maxRaw := strings.TrimSpace(os.Getenv(rtcUDPPortMaxEnv))
	if minRaw != "" || maxRaw != "" {
		if minRaw == "" || maxRaw == "" {
			return cfg, fmt.Errorf("%s and %s must be configured together", rtcUDPPortMinEnv, rtcUDPPortMaxEnv)
		}
		minPort, err := parseRTCUDPPort(rtcUDPPortMinEnv, minRaw)
		if err != nil {
			return cfg, err
		}
		maxPort, err := parseRTCUDPPort(rtcUDPPortMaxEnv, maxRaw)
		if err != nil {
			return cfg, err
		}
		cfg.UDPPortMin = minPort
		cfg.UDPPortMax = maxPort
	}

	for _, part := range strings.Split(os.Getenv(rtcNATIPsEnv), ",") {
		ip := strings.TrimSpace(part)
		if ip == "" {
			continue
		}
		if net.ParseIP(ip) == nil {
			return cfg, fmt.Errorf("%s contains invalid IP %q", rtcNATIPsEnv, ip)
		}
		cfg.NATIPs = append(cfg.NATIPs, ip)
	}
	if len(cfg.NATIPs) == 0 {
		for _, ip := range inferredNATIPs {
			ip = strings.TrimSpace(ip)
			if ip == "" {
				continue
			}
			if net.ParseIP(ip) == nil {
				return cfg, fmt.Errorf("inferred display WebRTC NAT IP %q is invalid", ip)
			}
			cfg.NATIPs = append(cfg.NATIPs, ip)
		}
	}
	return cfg, nil
}

func parseRTCUDPPort(name, raw string) (uint16, error) {
	port, err := strconv.ParseUint(raw, 10, 16)
	if err != nil || port == 0 {
		return 0, fmt.Errorf("%s must be a UDP port between 1 and 65535", name)
	}
	return uint16(port), nil
}

// negotiateCodec inspects the remote SDP offer's video m-section and returns
// the codec the encoder should produce. H264 is preferred whenever the offer
// advertises it; VP8 is used as a fallback. forceVP8 short-circuits the
// preference to VP8 (useful for environments without an x264 plugin) and
// errors out if the peer did not offer VP8 — silently encoding H264 in that
// situation would defeat the purpose of the override.
func negotiateCodec(offerSDP string, forceVP8 bool) (string, error) {
	offered := offeredVideoCodecs(offerSDP)
	if forceVP8 {
		if offered.vp8 {
			return CodecVP8, nil
		}
		return "", fmt.Errorf("%w: peer did not offer VP8 (force-VP8 enabled)", ErrCodecUnsupported)
	}
	if offered.h264 {
		return CodecH264, nil
	}
	if offered.vp8 {
		return CodecVP8, nil
	}
	return "", fmt.Errorf("%w: peer offered neither H264 nor VP8", ErrCodecUnsupported)
}

type offeredCodecs struct {
	h264 bool
	vp8  bool
}

func offeredVideoCodecs(rawSDP string) offeredCodecs {
	var result offeredCodecs
	parsed := &sdpv3.SessionDescription{}
	if err := parsed.Unmarshal([]byte(rawSDP)); err != nil {
		return result
	}
	for _, media := range parsed.MediaDescriptions {
		if media == nil || media.MediaName.Media != "video" {
			continue
		}
		for _, attr := range media.Attributes {
			if attr.Key != "rtpmap" {
				continue
			}
			value := strings.ToUpper(attr.Value)
			switch {
			case strings.Contains(value, "H264"):
				result.h264 = true
			case strings.Contains(value, "VP8"):
				result.vp8 = true
			}
		}
	}
	return result
}

func registerVideoCodec(engine *webrtc.MediaEngine, codec string) error {
	switch codec {
	case CodecH264:
		return engine.RegisterCodec(webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    webrtc.MimeTypeH264,
				ClockRate:   videoClockRate,
				SDPFmtpLine: h264FmtpLine,
			},
			PayloadType: videoPayloadTypeH264,
		}, webrtc.RTPCodecTypeVideo)
	case CodecVP8:
		return engine.RegisterCodec(webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  webrtc.MimeTypeVP8,
				ClockRate: videoClockRate,
			},
			PayloadType: videoPayloadTypeVP8,
		}, webrtc.RTPCodecTypeVideo)
	default:
		return fmt.Errorf("%w: %q", ErrCodecUnsupported, codec)
	}
}

func forceVP8FromEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(forceVP8Env))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func gstreamerArgs(codec string, rfbPort, rtpPort int) []string {
	base := []string{
		"-q",
		"rfbsrc", "host=127.0.0.1", fmt.Sprintf("port=%d", rfbPort), "shared=true", "incremental=true", "use-copyrect=true", "do-timestamp=true",
		"!", "videoconvert",
		"!", "videorate",
		"!", fmt.Sprintf("video/x-raw,framerate=%d/1", videoFrameRate),
		"!", "queue", "leaky=downstream", "max-size-buffers=2",
	}
	switch codec {
	case CodecH264:
		return append(base,
			"!", "x264enc", "tune=zerolatency", "speed-preset=ultrafast",
			"bframes=0", "key-int-max=30", "byte-stream=true",
			"!", "video/x-h264,profile=baseline,stream-format=byte-stream,alignment=au",
			"!", "h264parse", "config-interval=-1",
			"!", "rtph264pay", "aggregate-mode=zero-latency", "config-interval=-1",
			fmt.Sprintf("pt=%d", videoPayloadTypeH264),
			"!", "udpsink", "host=127.0.0.1", fmt.Sprintf("port=%d", rtpPort), "sync=false", "async=false",
		)
	case CodecVP8:
		fallthrough
	default:
		return append(base,
			"!", "vp8enc", "deadline=1", "cpu-used=8", "keyframe-max-dist=30",
			"!", "rtpvp8pay", fmt.Sprintf("pt=%d", videoPayloadTypeVP8),
			"!", "udpsink", "host=127.0.0.1", fmt.Sprintf("port=%d", rtpPort), "sync=false", "async=false",
		)
	}
}

func gstreamerScreenshotArgs(rfbPort int, outputPath string) []string {
	return []string{
		"-q",
		"rfbsrc", "host=127.0.0.1", fmt.Sprintf("port=%d", rfbPort), "shared=true", "incremental=false", "do-timestamp=true", "num-buffers=1",
		"!", "videoconvert",
		"!", "videoscale",
		"!", fmt.Sprintf("video/x-raw,width=%d,pixel-aspect-ratio=1/1", screenshotWidth),
		"!", "jpegenc", fmt.Sprintf("quality=%d", screenshotQuality),
		"!", "filesink", "location=" + outputPath,
	}
}

func limitJPEGSize(data []byte, maxBytes int) ([]byte, error) {
	if maxBytes <= 0 || len(data) <= maxBytes {
		return data, nil
	}
	source, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode oversized display screenshot: %w", err)
	}

	var last []byte
	sourceWidth := source.Bounds().Dx()
	for _, candidate := range screenshotJPEGCandidates {
		img := source
		if candidate.width > 0 && candidate.width < sourceWidth {
			img = resizeNearest(source, candidate.width)
		}
		encoded, err := encodeJPEG(img, candidate.quality)
		if err != nil {
			return nil, err
		}
		if len(encoded) <= maxBytes {
			return encoded, nil
		}
		last = encoded
	}

	return nil, fmt.Errorf("display screenshot exceeds size limit after compression: %d > %d bytes", len(last), maxBytes)
}

func encodeJPEG(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func resizeNearest(src image.Image, width int) image.Image {
	bounds := src.Bounds()
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()
	if width <= 0 || srcWidth <= 0 || srcHeight <= 0 || width >= srcWidth {
		return src
	}
	height := width * srcHeight / srcWidth
	if height < 1 {
		height = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		sourceY := bounds.Min.Y + y*srcHeight/height
		for x := 0; x < width; x++ {
			sourceX := bounds.Min.X + x*srcWidth/width
			dst.Set(x, y, src.At(sourceX, sourceY))
		}
	}
	return dst
}

func resolveGSTLaunch() (string, error) {
	if path := strings.TrimSpace(os.Getenv(gstLaunchEnv)); path != "" {
		return resolveExecutable(path)
	}

	candidates := []string{"gst-launch-1.0"}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates,
			"/opt/homebrew/bin/gst-launch-1.0",
			"/usr/local/bin/gst-launch-1.0",
		)
	}
	var errs []error
	for _, candidate := range candidates {
		path, err := resolveExecutable(candidate)
		if err == nil {
			return path, nil
		}
		errs = append(errs, err)
	}
	return "", errors.Join(errs...)
}

func resolveExecutable(candidate string) (string, error) {
	if strings.Contains(candidate, string(os.PathSeparator)) {
		cleanPath := filepath.Clean(candidate)
		info, err := os.Stat(cleanPath) //nolint:gosec // operator-controlled binary path from config/env.
		if err != nil {
			return "", err
		}
		if !isUsableExecutable(info, runtime.GOOS) {
			return "", fmt.Errorf("%s is not executable", cleanPath)
		}
		return cleanPath, nil
	}
	return exec.LookPath(candidate)
}

func isUsableExecutable(info os.FileInfo, goos string) bool {
	if info.IsDir() {
		return false
	}
	if goos == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

func isSocketReady(ctx context.Context, path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	dialCtx, cancel := context.WithTimeout(ctx, socketProbeTimeout)
	defer cancel()
	dialer := net.Dialer{Timeout: socketProbeTimeout}
	conn, err := dialer.DialContext(dialCtx, "unix", filepath.Clean(path))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

type processLogWriter struct {
	logger *slog.Logger
	botID  string
}

func (w processLogWriter) Write(p []byte) (int, error) {
	text := strings.TrimSpace(string(p))
	if text != "" {
		w.logger.Warn("display gstreamer output", slog.String("bot_id", w.botID), slog.String("message", text))
	}
	return len(p), nil
}

type rfbCursorWatcher struct {
	conn     net.Conn
	onCursor func(cursorMetadata)
}

type rfbServerInit struct {
	width  uint16
	height uint16
	format rfbPixelFormat
	name   string
}

type rfbPixelFormat struct {
	bitsPerPixel uint8
	depth        uint8
	bigEndian    bool
	trueColor    bool
	redMax       uint16
	greenMax     uint16
	blueMax      uint16
	redShift     uint8
	greenShift   uint8
	blueShift    uint8
}

type rfbRectHeader struct {
	x        uint16
	y        uint16
	width    uint16
	height   uint16
	encoding int32
}

var cursorPixelFormat = rfbPixelFormat{
	bitsPerPixel: 32,
	depth:        24,
	trueColor:    true,
	redMax:       255,
	greenMax:     255,
	blueMax:      255,
	redShift:     16,
	greenShift:   8,
	blueShift:    0,
}

func (w *rfbCursorWatcher) Run(ctx context.Context) error {
	if w == nil || w.conn == nil {
		return errors.New("RFB cursor watcher has no connection")
	}
	defer func() { _ = w.conn.Close() }()
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = w.conn.Close()
		case <-done:
		}
	}()
	defer close(done)

	init, err := handshakeRFBNoneSecurityServerInit(w.conn, true)
	if err != nil {
		return err
	}
	if init == nil || init.width == 0 || init.height == 0 {
		return errors.New("RFB server init is missing display size")
	}
	if err := writeRFBSetPixelFormat(w.conn, cursorPixelFormat); err != nil {
		return fmt.Errorf("write RFB cursor pixel format: %w", err)
	}
	if err := writeRFBSetEncodings(w.conn, []int32{rfbEncodingCursor, rfbEncodingXCursor, rfbEncodingRaw, rfbEncodingCopyRect}); err != nil {
		return fmt.Errorf("write RFB cursor encodings: %w", err)
	}

	incremental := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := writeRFBFramebufferUpdateRequest(w.conn, incremental, 0, 0, init.width, init.height); err != nil {
			return fmt.Errorf("request RFB cursor update: %w", err)
		}
		incremental = true
		if err := w.readServerMessage(); err != nil {
			return err
		}
	}
}

func (w *rfbCursorWatcher) readServerMessage() error {
	msgType := []byte{0}
	if _, err := io.ReadFull(w.conn, msgType); err != nil {
		return fmt.Errorf("read RFB server message type: %w", err)
	}
	switch msgType[0] {
	case 0:
		return w.readFramebufferUpdate()
	case 2:
		return nil
	case 3:
		return skipRFBCutText(w.conn)
	default:
		return fmt.Errorf("unsupported RFB server message type %d", msgType[0])
	}
}

func (w *rfbCursorWatcher) readFramebufferUpdate() error {
	header := make([]byte, 3)
	if _, err := io.ReadFull(w.conn, header); err != nil {
		return fmt.Errorf("read RFB framebuffer update header: %w", err)
	}
	count := int(binary.BigEndian.Uint16(header[1:3]))
	for i := 0; i < count; i++ {
		rect, err := readRFBRectHeader(w.conn)
		if err != nil {
			return err
		}
		switch rect.encoding {
		case rfbEncodingCursor:
			cursor, err := readRFBRichCursor(w.conn, rect, cursorPixelFormat)
			if err != nil {
				return err
			}
			if w.onCursor != nil {
				w.onCursor(cursor)
			}
		case rfbEncodingXCursor:
			cursor, err := readRFBXCursor(w.conn, rect)
			if err != nil {
				return err
			}
			if w.onCursor != nil {
				w.onCursor(cursor)
			}
		case rfbEncodingRaw:
			if err := skipRFBRectangle(w.conn, rect, cursorPixelFormat.bytesPerPixel()); err != nil {
				return err
			}
		case rfbEncodingCopyRect:
			if _, err := io.CopyN(io.Discard, w.conn, 4); err != nil {
				return fmt.Errorf("skip RFB copyrect payload: %w", err)
			}
		default:
			return fmt.Errorf("unsupported RFB rectangle encoding %d", rect.encoding)
		}
	}
	return nil
}

func readRFBRectHeader(r io.Reader) (rfbRectHeader, error) {
	buf := make([]byte, 12)
	if _, err := io.ReadFull(r, buf); err != nil {
		return rfbRectHeader{}, fmt.Errorf("read RFB rectangle header: %w", err)
	}
	return parseRFBRectHeader(buf), nil
}

func parseRFBRectHeader(buf []byte) rfbRectHeader {
	if len(buf) < 12 {
		return rfbRectHeader{}
	}
	return rfbRectHeader{
		x:        binary.BigEndian.Uint16(buf[0:2]),
		y:        binary.BigEndian.Uint16(buf[2:4]),
		width:    binary.BigEndian.Uint16(buf[4:6]),
		height:   binary.BigEndian.Uint16(buf[6:8]),
		encoding: int32(binary.BigEndian.Uint32(buf[8:12])), //nolint:gosec // RFB encodings are signed 32-bit values on the wire.
	}
}

func readRFBRichCursor(r io.Reader, rect rfbRectHeader, format rfbPixelFormat) (cursorMetadata, error) {
	if rect.width == 0 || rect.height == 0 {
		return cursorMetadata{Type: "cursor", Source: "rfb", Hidden: true}, nil
	}
	if rect.width > maxCursorDimension || rect.height > maxCursorDimension {
		return cursorMetadata{}, fmt.Errorf("RFB cursor is too large: %dx%d", rect.width, rect.height)
	}
	bytesPerPixel := format.bytesPerPixel()
	pixelBytes := int(rect.width) * int(rect.height) * bytesPerPixel
	maskBytes := rfbCursorMaskBytes(rect.width, rect.height)
	pixels := make([]byte, pixelBytes)
	if _, err := io.ReadFull(r, pixels); err != nil {
		return cursorMetadata{}, fmt.Errorf("read RFB cursor pixels: %w", err)
	}
	mask := make([]byte, maskBytes)
	if _, err := io.ReadFull(r, mask); err != nil {
		return cursorMetadata{}, fmt.Errorf("read RFB cursor mask: %w", err)
	}
	return richCursorMetadata(rect, pixels, mask, format)
}

func readRFBXCursor(r io.Reader, rect rfbRectHeader) (cursorMetadata, error) {
	if rect.width == 0 || rect.height == 0 {
		return cursorMetadata{Type: "cursor", Source: "rfb", Hidden: true}, nil
	}
	if rect.width > maxCursorDimension || rect.height > maxCursorDimension {
		return cursorMetadata{}, fmt.Errorf("RFB XCursor is too large: %dx%d", rect.width, rect.height)
	}
	maskBytes := rfbCursorMaskBytes(rect.width, rect.height)
	payload := make([]byte, 6+maskBytes*2)
	if _, err := io.ReadFull(r, payload); err != nil {
		return cursorMetadata{}, fmt.Errorf("read RFB XCursor payload: %w", err)
	}
	return cursorMetadataFromGeometry(rect), nil
}

func richCursorMetadata(rect rfbRectHeader, pixels, mask []byte, format rfbPixelFormat) (cursorMetadata, error) {
	if rect.width == 0 || rect.height == 0 {
		return cursorMetadata{Type: "cursor", Source: "rfb", Hidden: true}, nil
	}
	bytesPerPixel := format.bytesPerPixel()
	expectedPixels := int(rect.width) * int(rect.height) * bytesPerPixel
	expectedMask := rfbCursorMaskBytes(rect.width, rect.height)
	if len(pixels) != expectedPixels {
		return cursorMetadata{}, fmt.Errorf("RFB cursor pixel length = %d, want %d", len(pixels), expectedPixels)
	}
	if len(mask) != expectedMask {
		return cursorMetadata{}, fmt.Errorf("RFB cursor mask length = %d, want %d", len(mask), expectedMask)
	}
	return cursorMetadataFromGeometry(rect), nil
}

func cursorMetadataFromGeometry(rect rfbRectHeader) cursorMetadata {
	return cursorMetadata{
		Type:   "cursor",
		Source: "rfb",
		Cursor: nativeCursorType(rect),
		Width:  int(rect.width),
		Height: int(rect.height),
		HotX:   int(rect.x),
		HotY:   int(rect.y),
	}
}

func nativeCursorType(rect rfbRectHeader) string {
	width := int(rect.width)
	height := int(rect.height)
	hotX := int(rect.x)
	hotY := int(rect.y)
	if width <= 0 || height <= 0 {
		return ""
	}
	centerX := width / 2
	centerY := height / 2
	if width <= 18 && height >= 18 && absInt(hotX-centerX) <= 3 && absInt(hotY-centerY) <= 4 {
		return "text"
	}
	if width >= height*2 && absInt(hotY-centerY) <= 4 {
		return "ew-resize"
	}
	if height >= width*2 && absInt(hotX-centerX) <= 4 {
		return "ns-resize"
	}
	if width >= 18 && height >= 18 && absInt(hotX-centerX) <= 4 && absInt(hotY-centerY) <= 4 {
		return "move"
	}
	if width >= 16 && height >= 16 && hotX > 3 && hotY <= 5 {
		return "pointer"
	}
	return "default"
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func (f rfbPixelFormat) bytesPerPixel() int {
	if f.bitsPerPixel == 0 {
		return 0
	}
	return int(f.bitsPerPixel+7) / 8
}

func rfbCursorMaskBytes(width, height uint16) int {
	return int((width+7)/8) * int(height)
}

func writeRFBSetPixelFormat(w io.Writer, format rfbPixelFormat) error {
	msg := make([]byte, 20)
	msg[0] = 0
	msg[4] = format.bitsPerPixel
	msg[5] = format.depth
	if format.bigEndian {
		msg[6] = 1
	}
	if format.trueColor {
		msg[7] = 1
	}
	binary.BigEndian.PutUint16(msg[8:10], format.redMax)
	binary.BigEndian.PutUint16(msg[10:12], format.greenMax)
	binary.BigEndian.PutUint16(msg[12:14], format.blueMax)
	msg[14] = format.redShift
	msg[15] = format.greenShift
	msg[16] = format.blueShift
	_, err := w.Write(msg)
	return err
}

func writeRFBSetEncodings(w io.Writer, encodings []int32) error {
	if len(encodings) > 0xffff {
		return fmt.Errorf("too many RFB encodings: %d", len(encodings))
	}
	msg := make([]byte, 4+len(encodings)*4)
	msg[0] = 2
	binary.BigEndian.PutUint16(msg[2:4], uint16(len(encodings))) //nolint:gosec // Length is range-checked above.
	offset := 4
	for _, encoding := range encodings {
		binary.BigEndian.PutUint32(msg[offset:offset+4], uint32(encoding))
		offset += 4
	}
	_, err := w.Write(msg)
	return err
}

func writeRFBFramebufferUpdateRequest(w io.Writer, incremental bool, x, y, width, height uint16) error {
	msg := make([]byte, 10)
	msg[0] = 3
	if incremental {
		msg[1] = 1
	}
	binary.BigEndian.PutUint16(msg[2:4], x)
	binary.BigEndian.PutUint16(msg[4:6], y)
	binary.BigEndian.PutUint16(msg[6:8], width)
	binary.BigEndian.PutUint16(msg[8:10], height)
	_, err := w.Write(msg)
	return err
}

func skipRFBRectangle(r io.Reader, rect rfbRectHeader, bytesPerPixel int) error {
	if bytesPerPixel <= 0 {
		return errors.New("invalid RFB pixel format")
	}
	size := int64(rect.width) * int64(rect.height) * int64(bytesPerPixel)
	if size < 0 || size > maxRFBDiscardBytes {
		return fmt.Errorf("RFB rectangle payload too large: %d", size)
	}
	if _, err := io.CopyN(io.Discard, r, size); err != nil {
		return fmt.Errorf("skip RFB raw rectangle: %w", err)
	}
	return nil
}

func skipRFBCutText(r io.Reader) error {
	header := make([]byte, 7)
	if _, err := io.ReadFull(r, header); err != nil {
		return fmt.Errorf("read RFB cut text header: %w", err)
	}
	length := binary.BigEndian.Uint32(header[3:7])
	if length > maxRFBDiscardBytes {
		return fmt.Errorf("RFB cut text too large: %d", length)
	}
	if _, err := io.CopyN(io.Discard, r, int64(length)); err != nil {
		return fmt.Errorf("skip RFB cut text: %w", err)
	}
	return nil
}

type rfbInputClient struct {
	mu             sync.Mutex
	conn           net.Conn
	lastX          int
	lastY          int
	lastButtonMask uint8
	hasPointer     bool
}

func newRFBInputClient(conn net.Conn) (*rfbInputClient, error) {
	client := &rfbInputClient{conn: conn}
	if err := client.handshake(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return client, nil
}

func (c *rfbInputClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

func (c *rfbInputClient) Pointer(x, y int, buttonMask uint8) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return net.ErrClosed
	}
	c.lastX = x
	c.lastY = y
	c.lastButtonMask = buttonMask
	c.hasPointer = true
	return c.writePointerLocked(x, y, buttonMask)
}

func (c *rfbInputClient) writePointerLocked(x, y int, buttonMask uint8) error {
	msg := []byte{5, buttonMask, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(msg[2:4], clampUint16(x))
	binary.BigEndian.PutUint16(msg[4:6], clampUint16(y))
	_, err := c.conn.Write(msg)
	return err
}

func (c *rfbInputClient) Key(keysym uint32, down bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return net.ErrClosed
	}
	msg := []byte{4, 0, 0, 0, 0, 0, 0, 0}
	if down {
		msg[1] = 1
	}
	binary.BigEndian.PutUint32(msg[4:8], keysym)
	if c.hasPointer {
		jitterX := c.lastX - 1
		if jitterX < 0 {
			jitterX = c.lastX + 1
		}
		msg = append(msg, pointerEventBytes(jitterX, c.lastY, c.lastButtonMask)...)
		msg = append(msg, pointerEventBytes(c.lastX, c.lastY, c.lastButtonMask)...)
	}
	_, err := c.conn.Write(msg)
	return err
}

func pointerEventBytes(x, y int, buttonMask uint8) []byte {
	msg := []byte{5, buttonMask, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(msg[2:4], clampUint16(x))
	binary.BigEndian.PutUint16(msg[4:6], clampUint16(y))
	return msg
}

func (c *rfbInputClient) handshake() error {
	return handshakeRFBNoneSecurity(c.conn, true)
}

func probeRFBNoneSecurity(conn net.Conn) error {
	defer func() { _ = conn.Close() }()
	return handshakeRFBNoneSecurity(conn, true)
}

func handshakeRFBNoneSecurity(conn net.Conn, clientInit bool) error {
	_, err := handshakeRFBNoneSecurityServerInit(conn, clientInit)
	return err
}

func handshakeRFBNoneSecurityServerInit(conn net.Conn, clientInit bool) (*rfbServerInit, error) {
	if err := conn.SetDeadline(time.Now().Add(displayProbePeriod)); err != nil {
		return nil, err
	}
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	version := make([]byte, 12)
	if _, err := io.ReadFull(conn, version); err != nil {
		return nil, fmt.Errorf("read RFB version: %w", err)
	}
	if _, err := conn.Write(version); err != nil {
		return nil, fmt.Errorf("write RFB version: %w", err)
	}

	count := []byte{0}
	if _, err := io.ReadFull(conn, count); err != nil {
		return nil, fmt.Errorf("read RFB security types: %w", err)
	}
	if count[0] == 0 {
		reason, err := readRFBString(conn)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("RFB security negotiation failed: %s", reason)
	}
	types := make([]byte, int(count[0]))
	if _, err := io.ReadFull(conn, types); err != nil {
		return nil, fmt.Errorf("read RFB security type list: %w", err)
	}
	if !containsByte(types, 1) {
		return nil, errors.New("RFB server does not allow None security")
	}
	if _, err := conn.Write([]byte{1}); err != nil {
		return nil, fmt.Errorf("write RFB security type: %w", err)
	}
	result := make([]byte, 4)
	if _, err := io.ReadFull(conn, result); err != nil {
		return nil, fmt.Errorf("read RFB security result: %w", err)
	}
	if binary.BigEndian.Uint32(result) != 0 {
		reason, err := readRFBString(conn)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("RFB security rejected: %s", reason)
	}
	if !clientInit {
		return nil, nil
	}

	if _, err := conn.Write([]byte{1}); err != nil {
		return nil, fmt.Errorf("write RFB client init: %w", err)
	}
	header := make([]byte, 24)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, fmt.Errorf("read RFB server init: %w", err)
	}
	init := &rfbServerInit{
		width:  binary.BigEndian.Uint16(header[0:2]),
		height: binary.BigEndian.Uint16(header[2:4]),
		format: parseRFBPixelFormat(header[4:20]),
	}
	nameLen := binary.BigEndian.Uint32(header[20:24])
	if nameLen > 0 {
		if nameLen > 4096 {
			return nil, fmt.Errorf("RFB desktop name too large: %d", nameLen)
		}
		name := make([]byte, nameLen)
		if _, err := io.ReadFull(conn, name); err != nil {
			return nil, fmt.Errorf("read RFB server name: %w", err)
		}
		init.name = string(name)
	}
	return init, nil
}

func parseRFBPixelFormat(data []byte) rfbPixelFormat {
	if len(data) < 16 {
		return rfbPixelFormat{}
	}
	return rfbPixelFormat{
		bitsPerPixel: data[0],
		depth:        data[1],
		bigEndian:    data[2] != 0,
		trueColor:    data[3] != 0,
		redMax:       binary.BigEndian.Uint16(data[4:6]),
		greenMax:     binary.BigEndian.Uint16(data[6:8]),
		blueMax:      binary.BigEndian.Uint16(data[8:10]),
		redShift:     data[10],
		greenShift:   data[11],
		blueShift:    data[12],
	}
}

func readRFBString(r io.Reader) (string, error) {
	sizeBuf := make([]byte, 4)
	if _, err := io.ReadFull(r, sizeBuf); err != nil {
		return "", err
	}
	size := binary.BigEndian.Uint32(sizeBuf)
	if size == 0 {
		return "", nil
	}
	if size > 4096 {
		return "", fmt.Errorf("RFB string too large: %d", size)
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func containsByte(values []byte, target byte) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func clampUint16(value int) uint16 {
	switch {
	case value < 0:
		return 0
	case value > 0xffff:
		return 0xffff
	default:
		return uint16(value)
	}
}
