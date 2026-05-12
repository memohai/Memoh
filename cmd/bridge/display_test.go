package main

import "testing"

func TestDisplayGeometryDefaultsToFourThree(t *testing.T) {
	t.Setenv(displayGeometryEnv, "")

	if got := displayGeometry(); got != "1280x960" {
		t.Fatalf("displayGeometry() = %q, want 1280x960", got)
	}
}

func TestDisplayGeometryCanBeOverridden(t *testing.T) {
	t.Setenv(displayGeometryEnv, "1440x1080")

	if got := displayGeometry(); got != "1440x1080" {
		t.Fatalf("displayGeometry() = %q, want 1440x1080", got)
	}
}

func TestIsBrowserArgMatchesRealBrowserExecutables(t *testing.T) {
	t.Parallel()

	for _, arg := range []string{
		"chromium",
		"/usr/bin/chromium",
		"/usr/lib/chromium/chromium",
		"google-chrome-stable",
		"/opt/google/chrome/chrome",
	} {
		if !isBrowserArg(arg) {
			t.Fatalf("expected %q to be recognized as a browser executable", arg)
		}
	}
}

func TestIsBrowserArgRejectsShellCommandsContainingBrowserText(t *testing.T) {
	t.Parallel()

	for _, arg := range []string{
		"sh -lc command -v chromium",
		"--remote-debugging-port=9222",
		"/tmp/memoh-display-prepare.sh",
	} {
		if isBrowserArg(arg) {
			t.Fatalf("expected %q not to be recognized as a browser executable", arg)
		}
	}
}
