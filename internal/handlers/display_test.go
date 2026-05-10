package handlers

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestResolveDisplayHostIPsStripsPorts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host string
		want []string
	}{
		{name: "plain ipv4", host: "100.123.2.67", want: []string{"100.123.2.67"}},
		{name: "ipv4 port", host: "100.123.2.67:8082", want: []string{"100.123.2.67"}},
		{name: "bracketed ipv6 port", host: "[::1]:8082", want: []string{"::1"}},
		{name: "plain ipv6", host: "::1", want: []string{"::1"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveDisplayHostIPs(context.Background(), tt.host); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("resolveDisplayHostIPs(%q) = %#v, want %#v", tt.host, got, tt.want)
			}
		})
	}
}

func TestFirstHeaderValue(t *testing.T) {
	t.Parallel()

	got := firstHeaderValue("100.123.2.67, 10.0.0.2")
	if got != "100.123.2.67" {
		t.Fatalf("firstHeaderValue returned %q", got)
	}
}

func TestDisplayPrepareCommandInjectsInstallScript(t *testing.T) {
	t.Parallel()

	cmd := displayPrepareCommand()
	if !strings.Contains(cmd, "cat >/tmp/memoh-desktop-install.sh") {
		t.Fatal("display prepare command must inject the install script")
	}
	if !strings.Contains(cmd, ". /tmp/memoh-desktop-install.sh") {
		t.Fatal("display prepare command must source the injected install script")
	}
	if !strings.Contains(cmd, "install_debian()") || !strings.Contains(cmd, "install_alpine()") {
		t.Fatal("injected install script must define Debian and Alpine installers")
	}
	if strings.Contains(displayPrepareMainCommand, "apt-get install") || strings.Contains(displayPrepareMainCommand, "apk add") {
		t.Fatal("package installation details should stay in scripts/desktop-install.sh")
	}
	if strings.Contains(displayPrepareMainCommand, "set -- $(tr") {
		t.Fatal("Xvnc process detection must not word-split shell command lines")
	}
	if !strings.Contains(displayPrepareMainCommand, "grep -Eq '(^|/)Xvnc$'") || !strings.Contains(displayPrepareMainCommand, "grep -Fxq ':99'") {
		t.Fatal("Xvnc process detection must match real Xvnc processes on display :99")
	}
	if !strings.Contains(displayPrepareMainCommand, "grep -Eq '(^|/)(google-chrome-stable|google-chrome|chromium|chromium-browser|chrome)$'") {
		t.Fatal("browser process detection must match real browser argv entries only")
	}
	if !strings.Contains(displayPrepareMainCommand, "grep -Eq '^--type=' && continue") {
		t.Fatal("CDP readiness detection must ignore Chromium child processes")
	}
	if !strings.Contains(displayPrepareMainCommand, "SingletonLock") {
		t.Fatal("display prepare must clean stale Chromium profile locks before starting the browser")
	}
	if strings.Contains(displayPrepareMainCommand, "rfbunixpath") || strings.Contains(displayPrepareMainCommand, "RFB_SOCKET") {
		t.Fatal("display prepare should use loopback TCP VNC instead of a bind-mounted Unix RFB socket")
	}
	if !strings.Contains(displayPrepareMainCommand, "-localhost -rfbport \"$RFB_PORT\"") {
		t.Fatal("display prepare must keep VNC on container loopback")
	}
}
