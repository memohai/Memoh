package display

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
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
)

var (
	ErrManagerUnavailable = errors.New("manager not configured")
	ErrDisplayDisabled    = errors.New("display disabled")
	ErrDisplayUnavailable = errors.New("display server not reachable")
	ErrEncoderUnavailable = errors.New("gstreamer unavailable")
	ErrCodecUnsupported   = errors.New("no compatible video codec offered")
)

type Workspace interface {
	BotDisplayEnabled(ctx context.Context, botID string) bool
	DisplaySocketPath(botID string) string
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
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

type OfferResponse struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
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
}

func NewService(logger *slog.Logger, workspace Workspace) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{
		logger:    logger.With(slog.String("component", "display")),
		workspace: workspace,
		sessions:  make(map[string]*session),
	}
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

	socketPath := s.workspace.DisplaySocketPath(botID)
	status.Running = isSocketReady(socketPath)
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

	socketPath := s.workspace.DisplaySocketPath(botID)
	if !isSocketReady(socketPath) {
		return OfferResponse{}, fmt.Errorf("%w: %s", ErrDisplayUnavailable, socketPath)
	}
	gstLaunch, err := resolveGSTLaunch()
	if err != nil {
		return OfferResponse{}, errors.Join(ErrEncoderUnavailable, err)
	}

	codec, err := negotiateCodec(req.SDP, forceVP8FromEnv())
	if err != nil {
		return OfferResponse{}, err
	}

	sess, err := s.session(ctx, botID, socketPath, gstLaunch, codec)
	if err != nil {
		return OfferResponse{}, err
	}
	return sess.answer(ctx, req)
}

func (s *Service) session(ctx context.Context, botID, socketPath, gstLaunch, codec string) (*session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sess := s.sessions[botID]; sess != nil && !sess.closed() {
		// Display sessions are shared across viewers via RTP fan-out. If a new
		// viewer needs a different codec, we refuse rather than tearing down
		// the existing pipeline — that would black out anyone already watching.
		if sess.codec != codec {
			return nil, fmt.Errorf("%w: another viewer is already using %s", ErrCodecUnsupported, sess.codec)
		}
		return sess, nil
	}

	sess := newSession(s, botID, socketPath, gstLaunch, codec)
	if err := sess.start(ctx); err != nil {
		sess.stop()
		return nil, err
	}
	s.sessions[botID] = sess
	return sess, nil
}

func (s *Service) removeSession(botID string, target *session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if current := s.sessions[botID]; current == target {
		delete(s.sessions, botID)
	}
}

type session struct {
	service    *Service
	botID      string
	socketPath string
	gstLaunch  string
	codec      string

	ctx          context.Context
	cancel       context.CancelFunc
	runCtxCancel context.CancelFunc

	proxy net.Listener
	udp   *net.UDPConn
	cmd   *exec.Cmd

	tracksMu sync.RWMutex
	tracks   map[string]*webrtc.TrackLocalStaticRTP
	input    *rfbInputClient

	stopOnce sync.Once
}

func newSession(service *Service, botID, socketPath, gstLaunch, codec string) *session {
	ctx, cancel := context.WithCancel(context.Background())
	return &session{
		service:    service,
		botID:      botID,
		socketPath: filepath.Clean(socketPath),
		gstLaunch:  gstLaunch,
		codec:      codec,
		ctx:        ctx,
		cancel:     cancel,
		tracks:     make(map[string]*webrtc.TrackLocalStaticRTP),
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
	if !isSocketReady(s.socketPath) {
		return fmt.Errorf("%w: %s", ErrDisplayUnavailable, s.socketPath)
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
	cmd.Stdout = processLogWriter{logger: s.service.logger, botID: s.botID}
	cmd.Stderr = processLogWriter{logger: s.service.logger, botID: s.botID}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start gstreamer display pipeline: %w", err)
	}
	s.cmd = cmd
	if input, err := newRFBInputClient(runCtx, s.socketPath); err == nil {
		s.input = input
	} else {
		s.service.logger.Warn("display input channel unavailable", slog.String("bot_id", s.botID), slog.Any("error", err))
	}
	s.runCtxCancel = runCtxCancel
	cancelRunCtx = false

	s.service.logger.Info("display encoder started",
		slog.String("bot_id", s.botID),
		slog.String("socket_path", s.socketPath),
		slog.String("gst_launch", s.gstLaunch),
		slog.String("codec", s.codec),
		slog.Int("proxy_port", proxyPort),
		slog.Int("rtp_port", rtpPort),
		slog.Int("pid", cmd.Process.Pid),
	)

	go s.acceptProxy()
	go s.forwardRTP()
	go s.waitGStreamer()

	select {
	case <-ctx.Done():
		return ctx.Err()
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

	mediaEngine := &webrtc.MediaEngine{}
	if err := registerVideoCodec(mediaEngine, s.codec); err != nil {
		return OfferResponse{}, err
	}

	api, rtcCfg, err := newWebRTCAPI(mediaEngine)
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

	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		if dc.Label() != "display-input" {
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

	var cleanupOnce sync.Once
	cleanup := func(closePeer bool) {
		cleanupOnce.Do(func() {
			s.removeTrack(trackID)
			if closePeer {
				_ = pc.Close()
			}
		})
	}

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		s.service.logger.Info("display webrtc connection state",
			slog.String("bot_id", s.botID),
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

	return OfferResponse{Type: "answer", SDP: local.SDP}, nil
}

type inputEvent struct {
	Type       string `json:"type"`
	X          int    `json:"x,omitempty"`
	Y          int    `json:"y,omitempty"`
	ButtonMask uint8  `json:"button_mask,omitempty"`
	Keysym     uint32 `json:"keysym,omitempty"`
	Down       bool   `json:"down,omitempty"`
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
	defer func() { _ = conn.Close() }()

	dialer := net.Dialer{Timeout: displayProbePeriod}
	rfbConn, err := dialer.DialContext(s.ctx, "unix", s.socketPath)
	if err != nil {
		s.service.logger.Warn("display RFB socket dial failed", slog.String("bot_id", s.botID), slog.String("socket_path", s.socketPath), slog.Any("error", err))
		return
	}
	defer func() { _ = rfbConn.Close() }()

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

func (s *session) waitGStreamer() {
	err := s.cmd.Wait()
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

func newWebRTCAPI(mediaEngine *webrtc.MediaEngine) (*webrtc.API, rtcSettings, error) {
	cfg, err := readRTCSettings()
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

func readRTCSettings() (rtcSettings, error) {
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
		"rfbsrc", "host=127.0.0.1", fmt.Sprintf("port=%d", rfbPort), "shared=true", "incremental=false", "do-timestamp=true",
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
		if info.IsDir() || info.Mode().Perm()&0o111 == 0 {
			return "", fmt.Errorf("%s is not executable", cleanPath)
		}
		return cleanPath, nil
	}
	return exec.LookPath(candidate)
}

func isSocketReady(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	info, err := os.Stat(filepath.Clean(path))
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSocket != 0
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

type rfbInputClient struct {
	mu   sync.Mutex
	conn net.Conn
}

func newRFBInputClient(ctx context.Context, socketPath string) (*rfbInputClient, error) {
	dialCtx, cancel := context.WithTimeout(ctx, displayProbePeriod)
	defer cancel()
	dialer := net.Dialer{Timeout: displayProbePeriod}
	conn, err := dialer.DialContext(dialCtx, "unix", socketPath)
	if err != nil {
		return nil, err
	}
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
	_, err := c.conn.Write(msg)
	return err
}

func (c *rfbInputClient) handshake() error {
	if err := c.conn.SetDeadline(time.Now().Add(displayProbePeriod)); err != nil {
		return err
	}
	defer func() { _ = c.conn.SetDeadline(time.Time{}) }()

	version := make([]byte, 12)
	if _, err := io.ReadFull(c.conn, version); err != nil {
		return fmt.Errorf("read RFB version: %w", err)
	}
	if _, err := c.conn.Write(version); err != nil {
		return fmt.Errorf("write RFB version: %w", err)
	}

	count := []byte{0}
	if _, err := io.ReadFull(c.conn, count); err != nil {
		return fmt.Errorf("read RFB security types: %w", err)
	}
	if count[0] == 0 {
		reason, err := readRFBString(c.conn)
		if err != nil {
			return err
		}
		return fmt.Errorf("RFB security negotiation failed: %s", reason)
	}
	types := make([]byte, int(count[0]))
	if _, err := io.ReadFull(c.conn, types); err != nil {
		return fmt.Errorf("read RFB security type list: %w", err)
	}
	if !containsByte(types, 1) {
		return errors.New("RFB server does not allow None security")
	}
	if _, err := c.conn.Write([]byte{1}); err != nil {
		return fmt.Errorf("write RFB security type: %w", err)
	}
	result := make([]byte, 4)
	if _, err := io.ReadFull(c.conn, result); err != nil {
		return fmt.Errorf("read RFB security result: %w", err)
	}
	if binary.BigEndian.Uint32(result) != 0 {
		reason, err := readRFBString(c.conn)
		if err != nil {
			return err
		}
		return fmt.Errorf("RFB security rejected: %s", reason)
	}

	if _, err := c.conn.Write([]byte{1}); err != nil {
		return fmt.Errorf("write RFB client init: %w", err)
	}
	header := make([]byte, 24)
	if _, err := io.ReadFull(c.conn, header); err != nil {
		return fmt.Errorf("read RFB server init: %w", err)
	}
	nameLen := binary.BigEndian.Uint32(header[20:24])
	if nameLen > 0 {
		if _, err := io.CopyN(io.Discard, c.conn, int64(nameLen)); err != nil {
			return fmt.Errorf("read RFB server name: %w", err)
		}
	}
	return nil
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
