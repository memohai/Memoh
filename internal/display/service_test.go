package display

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
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

func TestGStreamerArgsH264UsesX264AndH264Pay(t *testing.T) {
	args := gstreamerArgs(CodecH264, 5901, 5004)
	if !containsString(args, "incremental=false") {
		t.Fatal("rfbsrc must request a full initial frame")
	}
	if !containsString(args, "do-timestamp=true") {
		t.Fatal("rfbsrc buffers must be timestamped for RTP encoding")
	}
	if !containsString(args, "x264enc") {
		t.Fatal("H264 pipeline must use x264enc")
	}
	if !containsString(args, "rtph264pay") {
		t.Fatal("H264 pipeline must use rtph264pay")
	}
}

func TestGStreamerArgsVP8FallbackUsesVP8Pay(t *testing.T) {
	args := gstreamerArgs(CodecVP8, 5901, 5004)
	if !containsString(args, "vp8enc") {
		t.Fatal("VP8 pipeline must use vp8enc")
	}
	if !containsString(args, "rtpvp8pay") {
		t.Fatal("VP8 pipeline must use rtpvp8pay")
	}
}

func TestNegotiateCodecPrefersH264(t *testing.T) {
	// SDP fragment offering both H264 (PT 102) and VP8 (PT 96).
	offer := "v=0\r\n" +
		"o=- 0 0 IN IP4 127.0.0.1\r\n" +
		"s=-\r\n" +
		"t=0 0\r\n" +
		"m=video 9 UDP/TLS/RTP/SAVPF 102 96\r\n" +
		"c=IN IP4 0.0.0.0\r\n" +
		"a=rtpmap:102 H264/90000\r\n" +
		"a=rtpmap:96 VP8/90000\r\n"
	codec, err := negotiateCodec(offer, false)
	if err != nil {
		t.Fatalf("negotiateCodec returned error: %v", err)
	}
	if codec != CodecH264 {
		t.Fatalf("expected H264, got %s", codec)
	}
}

func TestNegotiateCodecFallsBackToVP8(t *testing.T) {
	offer := "v=0\r\n" +
		"o=- 0 0 IN IP4 127.0.0.1\r\n" +
		"s=-\r\n" +
		"t=0 0\r\n" +
		"m=video 9 UDP/TLS/RTP/SAVPF 96\r\n" +
		"c=IN IP4 0.0.0.0\r\n" +
		"a=rtpmap:96 VP8/90000\r\n"
	codec, err := negotiateCodec(offer, false)
	if err != nil {
		t.Fatalf("negotiateCodec returned error: %v", err)
	}
	if codec != CodecVP8 {
		t.Fatalf("expected VP8, got %s", codec)
	}
}

func TestNegotiateCodecForceVP8(t *testing.T) {
	offer := "v=0\r\n" +
		"o=- 0 0 IN IP4 127.0.0.1\r\n" +
		"s=-\r\n" +
		"t=0 0\r\n" +
		"m=video 9 UDP/TLS/RTP/SAVPF 102 96\r\n" +
		"c=IN IP4 0.0.0.0\r\n" +
		"a=rtpmap:102 H264/90000\r\n" +
		"a=rtpmap:96 VP8/90000\r\n"
	codec, err := negotiateCodec(offer, true)
	if err != nil {
		t.Fatalf("negotiateCodec returned error: %v", err)
	}
	if codec != CodecVP8 {
		t.Fatalf("expected forced VP8, got %s", codec)
	}
}

func TestNegotiateCodecForceVP8RejectsH264Only(t *testing.T) {
	offer := "v=0\r\n" +
		"o=- 0 0 IN IP4 127.0.0.1\r\n" +
		"s=-\r\n" +
		"t=0 0\r\n" +
		"m=video 9 UDP/TLS/RTP/SAVPF 102\r\n" +
		"c=IN IP4 0.0.0.0\r\n" +
		"a=rtpmap:102 H264/90000\r\n"
	if _, err := negotiateCodec(offer, true); err == nil {
		t.Fatal("force-VP8 must not silently fall back to H264")
	}
}

func TestNegotiateCodecNoMatch(t *testing.T) {
	offer := "v=0\r\n" +
		"o=- 0 0 IN IP4 127.0.0.1\r\n" +
		"s=-\r\n" +
		"t=0 0\r\n" +
		"m=video 9 UDP/TLS/RTP/SAVPF 100\r\n" +
		"c=IN IP4 0.0.0.0\r\n" +
		"a=rtpmap:100 AV1/90000\r\n"
	if _, err := negotiateCodec(offer, false); err == nil {
		t.Fatal("expected codec negotiation to fail without H264/VP8")
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
