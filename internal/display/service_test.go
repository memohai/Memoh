package display

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func TestReadRTCSettings(t *testing.T) {
	t.Setenv(rtcUDPPortMinEnv, "30000")
	t.Setenv(rtcUDPPortMaxEnv, "30100")
	t.Setenv(rtcNATIPsEnv, "127.0.0.1, 10.0.0.10")

	cfg, err := readRTCSettings(nil)
	if err != nil {
		t.Fatalf("readRTCSettings returned error: %v", err)
	}
	if cfg.UDPPortMin != 30000 || cfg.UDPPortMax != 30100 {
		t.Fatalf("unexpected UDP range: %d-%d", cfg.UDPPortMin, cfg.UDPPortMax)
	}
	if len(cfg.NATIPs) != 2 || cfg.NATIPs[0] != "127.0.0.1" || cfg.NATIPs[1] != "10.0.0.10" {
		t.Fatalf("unexpected NAT IPs: %#v", cfg.NATIPs)
	}
}

func TestIsSocketReadyRequiresListener(t *testing.T) {
	path := filepath.Join(os.TempDir(), "memoh-display-ready-test.sock")
	_ = os.Remove(path)
	t.Cleanup(func() { _ = os.Remove(path) })
	listenConfig := net.ListenConfig{}
	listener, err := listenConfig.Listen(context.Background(), "unix", path)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}
	if !isSocketReady(context.Background(), path) {
		t.Fatal("expected active unix socket to be ready")
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	if isSocketReady(context.Background(), path) {
		t.Fatal("closed unix socket file must not be reported ready")
	}
}

func TestReadRTCSettingsRejectsPartialPortRange(t *testing.T) {
	t.Setenv(rtcUDPPortMinEnv, "30000")

	if _, err := readRTCSettings(nil); err == nil {
		t.Fatal("expected partial port range to fail")
	}
}

func TestReadRTCSettingsRejectsInvalidNATIP(t *testing.T) {
	t.Setenv(rtcNATIPsEnv, "localhost")

	if _, err := readRTCSettings(nil); err == nil {
		t.Fatal("expected invalid NAT IP to fail")
	}
}

func TestReadRTCSettingsUsesInferredNATIPs(t *testing.T) {
	cfg, err := readRTCSettings([]string{"100.123.2.67", "10.0.0.2"})
	if err != nil {
		t.Fatalf("readRTCSettings returned error: %v", err)
	}
	if len(cfg.NATIPs) != 2 || cfg.NATIPs[0] != "100.123.2.67" || cfg.NATIPs[1] != "10.0.0.2" {
		t.Fatalf("unexpected inferred NAT IPs: %#v", cfg.NATIPs)
	}
}

func TestSessionReplacingPeerKeepsNewPeer(t *testing.T) {
	sess := newSession(NewService(nil, nil), "bot-1", "gst-launch-1.0", CodecVP8)
	first := &peerSession{id: "viewer-1", trackID: "track-1", state: "connected"}
	second := &peerSession{id: "viewer-1", trackID: "track-2", state: "new"}

	sess.addPeer(first)
	sess.addPeer(second)
	sess.removePeer(first)

	if got := sess.peer("viewer-1"); got != second {
		t.Fatal("closing a replaced peer must not remove the newer peer for the same display session")
	}
}

func TestSessionCloseStalePeers(t *testing.T) {
	sess := newSession(NewService(nil, nil), "bot-1", "gst-launch-1.0", CodecVP8)
	fresh := &peerSession{id: "fresh", createdAt: time.Now(), state: webrtc.PeerConnectionStateConnecting.String()}
	staleConnecting := &peerSession{id: "stale-connecting", createdAt: time.Now().Add(-stalePeerTTL - time.Second), state: webrtc.PeerConnectionStateConnecting.String()}
	staleDisconnected := &peerSession{id: "stale-disconnected", createdAt: time.Now(), state: webrtc.PeerConnectionStateDisconnected.String()}
	staleClosed := make(map[string]bool)
	staleConnecting.close = func() {
		staleClosed[staleConnecting.id] = true
		sess.removePeer(staleConnecting)
	}
	staleDisconnected.close = func() {
		staleClosed[staleDisconnected.id] = true
		sess.removePeer(staleDisconnected)
	}

	sess.addPeer(fresh)
	sess.addPeer(staleConnecting)
	sess.addPeer(staleDisconnected)
	sess.closeStalePeers(time.Now())

	if sess.peer("fresh") == nil {
		t.Fatal("fresh connecting peer should remain active")
	}
	if sess.peer("stale-connecting") != nil {
		t.Fatal("old connecting peer should be removed")
	}
	if sess.peer("stale-disconnected") != nil {
		t.Fatal("disconnected peer should be removed")
	}
	if !staleClosed["stale-connecting"] || !staleClosed["stale-disconnected"] {
		t.Fatal("stale peers should be closed through their close callbacks")
	}
}

func TestSessionWaitsForInFlightStart(t *testing.T) {
	service := NewService(nil, nil)
	start := &sessionStart{done: make(chan struct{})}
	service.starting["bot-1"] = start
	expected := newSession(service, "bot-1", "gst-launch-1.0", CodecVP8)

	done := make(chan struct {
		sess *session
		err  error
	}, 1)
	go func() {
		sess, err := service.session(context.Background(), "bot-1", "gst-launch-1.0", CodecVP8)
		done <- struct {
			sess *session
			err  error
		}{sess: sess, err: err}
	}()

	select {
	case <-done:
		t.Fatal("session should wait for the in-flight encoder start")
	case <-time.After(20 * time.Millisecond):
	}

	service.finishSessionStart("bot-1", start, expected, nil)

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("session returned error: %v", result.err)
		}
		if result.sess != expected {
			t.Fatal("session should reuse the encoder from the in-flight start")
		}
	case <-time.After(time.Second):
		t.Fatal("session did not return after in-flight start completed")
	}
}

func TestLimitJPEGSizeRecompressesOversizedImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1280, 800))
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 17) % 256),
				G: uint8((y * 31) % 256),
				B: uint8((x*y + y) % 256),
				A: 255,
			})
		}
	}

	var original bytes.Buffer
	if err := jpeg.Encode(&original, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("encode original jpeg: %v", err)
	}

	const maxBytes = 32 * 1024
	bounded, err := limitJPEGSize(original.Bytes(), maxBytes)
	if err != nil {
		t.Fatalf("limitJPEGSize returned error: %v", err)
	}
	if len(bounded) > maxBytes {
		t.Fatalf("bounded image is too large: %d > %d", len(bounded), maxBytes)
	}
	if _, err := jpeg.Decode(bytes.NewReader(bounded)); err != nil {
		t.Fatalf("bounded image must remain decodable JPEG: %v", err)
	}
}

func TestNegotiateCodec(t *testing.T) {
	h264AndVP8Offer := "v=0\r\n" +
		"o=- 0 0 IN IP4 127.0.0.1\r\n" +
		"s=-\r\n" +
		"t=0 0\r\n" +
		"m=video 9 UDP/TLS/RTP/SAVPF 102 96\r\n" +
		"c=IN IP4 0.0.0.0\r\n" +
		"a=rtpmap:102 H264/90000\r\n" +
		"a=rtpmap:96 VP8/90000\r\n"
	vp8OnlyOffer := "v=0\r\n" +
		"o=- 0 0 IN IP4 127.0.0.1\r\n" +
		"s=-\r\n" +
		"t=0 0\r\n" +
		"m=video 9 UDP/TLS/RTP/SAVPF 96\r\n" +
		"c=IN IP4 0.0.0.0\r\n" +
		"a=rtpmap:96 VP8/90000\r\n"
	h264OnlyOffer := "v=0\r\n" +
		"o=- 0 0 IN IP4 127.0.0.1\r\n" +
		"s=-\r\n" +
		"t=0 0\r\n" +
		"m=video 9 UDP/TLS/RTP/SAVPF 102\r\n" +
		"c=IN IP4 0.0.0.0\r\n" +
		"a=rtpmap:102 H264/90000\r\n"
	av1OnlyOffer := "v=0\r\n" +
		"o=- 0 0 IN IP4 127.0.0.1\r\n" +
		"s=-\r\n" +
		"t=0 0\r\n" +
		"m=video 9 UDP/TLS/RTP/SAVPF 100\r\n" +
		"c=IN IP4 0.0.0.0\r\n" +
		"a=rtpmap:100 AV1/90000\r\n"

	tests := []struct {
		name      string
		offer     string
		forceVP8  bool
		wantCodec string
		wantErr   bool
	}{
		{
			name:      "prefers H264",
			offer:     h264AndVP8Offer,
			wantCodec: CodecH264,
		},
		{
			name:      "falls back to VP8",
			offer:     vp8OnlyOffer,
			wantCodec: CodecVP8,
		},
		{
			name:      "force VP8",
			offer:     h264AndVP8Offer,
			forceVP8:  true,
			wantCodec: CodecVP8,
		},
		{
			name:     "force VP8 rejects H264 only",
			offer:    h264OnlyOffer,
			forceVP8: true,
			wantErr:  true,
		},
		{
			name:    "no match",
			offer:   av1OnlyOffer,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			codec, err := negotiateCodec(tt.offer, tt.forceVP8)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected codec negotiation to fail")
				}
				return
			}
			if err != nil {
				t.Fatalf("negotiateCodec returned error: %v", err)
			}
			if codec != tt.wantCodec {
				t.Fatalf("expected %s, got %s", tt.wantCodec, codec)
			}
		})
	}
}

func TestIsUsableExecutableAllowsWindowsFilesWithoutExecuteBits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gst-launch-1.0.exe")
	if err := os.WriteFile(path, []byte("fake exe"), 0o600); err != nil {
		t.Fatalf("write test executable: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat test executable: %v", err)
	}
	if !isUsableExecutable(info, "windows") {
		t.Fatal("Windows absolute executable paths must not require Unix execute bits")
	}
	if isUsableExecutable(info, "linux") {
		t.Fatal("non-Windows executable paths must still require Unix execute bits")
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat temp dir: %v", err)
	}
	if isUsableExecutable(dirInfo, "windows") {
		t.Fatal("directories must not be treated as Windows executables")
	}
}

func TestSessionRemoveTrackDefersEncoderStopUntilIdleHold(t *testing.T) {
	sess := newSession(NewService(nil, nil), "bot-1", "gst-launch-1.0", CodecVP8)
	trackID := "track-1"
	sess.addTrack(trackID, nil)

	sess.removeTrack(trackID)

	select {
	case <-sess.ctx.Done():
		t.Fatal("encoder must stay warm briefly after the last viewer disconnects")
	case <-time.After(20 * time.Millisecond):
	}
}
