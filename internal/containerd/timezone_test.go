package containerd

import (
	"os"
	"testing"
)

func TestTimezoneSpec_WithTZ(t *testing.T) {
	t.Setenv("TZ", "Asia/Shanghai")
	mounts, env := TimezoneSpec()
	if _, err := os.Stat("/etc/localtime"); err == nil {
		if len(mounts) < 1 {
			t.Fatal("expected at least one mount when /etc/localtime exists")
		}
	}
	if len(env) == 0 {
		t.Fatal("expected at least one env var when TZ is set")
	}
}

func TestTimezoneSpec_WithoutTZ(t *testing.T) {
	t.Setenv("TZ", "")
	mounts, env := TimezoneSpec()
	if len(env) != 0 {
		t.Fatalf("expected no env when TZ empty, got %d", len(env))
	}
	if _, err := os.Stat("/etc/localtime"); err != nil && len(mounts) != 0 {
		t.Fatalf("expected no mounts when /etc/localtime absent and TZ empty, got %d", len(mounts))
	}
}
