package containerd

import (
	"testing"
)

func TestTimezoneSpec_WithTZ(t *testing.T) {
	t.Setenv("TZ", "Asia/Shanghai")
	mounts, env := TimezoneSpec()
	if len(mounts) != 0 {
		t.Fatalf("expected no mounts, got %d", len(mounts))
	}
	if len(env) == 0 {
		t.Fatal("expected at least one env var when TZ is set")
	}
}

func TestTimezoneSpec_WithoutTZ(t *testing.T) {
	t.Setenv("TZ", "")
	mounts, _ := TimezoneSpec()
	if len(mounts) != 0 {
		t.Fatalf("expected no mounts, got %d", len(mounts))
	}
}
