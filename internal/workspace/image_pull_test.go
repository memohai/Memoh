package workspace

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/config"
	ctr "github.com/memohai/memoh/internal/container"
)

func TestPrepareImageForCreatePolicies(t *testing.T) {
	for _, tc := range []struct {
		name         string
		policy       string
		getImageErr  error
		wantMode     ImagePrepareMode
		wantGetCalls int
		wantPulls    int
	}{
		{
			name:         "if_not_present_skips_existing_image",
			policy:       config.ImagePullPolicyIfNotPresent,
			wantMode:     ImagePrepareSkipped,
			wantGetCalls: 1,
		},
		{
			name:         "if_not_present_pulls_missing_image",
			policy:       config.ImagePullPolicyIfNotPresent,
			getImageErr:  ctr.ErrNotFound,
			wantMode:     ImagePreparePulled,
			wantGetCalls: 1,
			wantPulls:    1,
		},
		{
			name:      "always_pulls",
			policy:    config.ImagePullPolicyAlways,
			wantMode:  ImagePreparePulled,
			wantPulls: 1,
		},
		{
			name:     "never_skips",
			policy:   config.ImagePullPolicyNever,
			wantMode: ImagePrepareSkipped,
		},
		{
			name:         "delegates_when_image_service_unsupported",
			getImageErr:  ctr.ErrNotSupported,
			wantMode:     ImagePrepareDelegated,
			wantGetCalls: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &legacyRouteTestService{getImageErr: tc.getImageErr}
			m := newLegacyRouteTestManager(t, svc, config.WorkspaceConfig{
				ImagePullPolicy: tc.policy,
			})

			result, err := m.PrepareImageForCreate(context.Background(), "debian:bookworm-slim", nil)
			if err != nil {
				t.Fatalf("PrepareImageForCreate returned error: %v", err)
			}
			if result.Mode != tc.wantMode {
				t.Fatalf("mode = %s, want %s", result.Mode, tc.wantMode)
			}
			if svc.getImageCalls != tc.wantGetCalls || svc.pullCalls != tc.wantPulls {
				t.Fatalf("unexpected calls: get=%d pull=%d", svc.getImageCalls, svc.pullCalls)
			}
		})
	}
}

func TestPrepareImageForCreatePullsThroughRuntimeRouter(t *testing.T) {
	svc := &legacyRouteTestService{getImageErr: ctr.ErrNotFound}
	router := NewRuntimeRouter(svc, nil)
	m := newLegacyRouteTestManager(t, router, config.WorkspaceConfig{
		ImagePullPolicy: config.ImagePullPolicyIfNotPresent,
	})

	result, err := m.PrepareImageForCreate(context.Background(), "debian:bookworm-slim", nil)
	if err != nil {
		t.Fatalf("PrepareImageForCreate returned error: %v", err)
	}
	if result.Mode != ImagePreparePulled {
		t.Fatalf("expected pulled, got %s", result.Mode)
	}
	if svc.getImageCalls != 1 || svc.pullCalls != 1 {
		t.Fatalf("unexpected calls: get=%d pull=%d", svc.getImageCalls, svc.pullCalls)
	}
}

func TestPrepareImageForCreateFallsBackToWorkspaceMirror(t *testing.T) {
	primary := "docker.io/memohai/workspace:debian"
	fallback := "memoh.cn/memohai/workspace:debian"
	svc := &legacyRouteTestService{
		getImageErr: ctr.ErrNotFound,
		pullErrs: map[string]error{
			primary: ctr.ErrRuntime,
		},
	}
	m := newLegacyRouteTestManager(t, svc, config.WorkspaceConfig{
		ImagePullPolicy: config.ImagePullPolicyIfNotPresent,
	})

	result, err := m.PrepareImageForCreate(context.Background(), "memohai/workspace:debian", nil)
	if err != nil {
		t.Fatalf("PrepareImageForCreate returned error: %v", err)
	}
	if result.Mode != ImagePreparePulled {
		t.Fatalf("expected pulled, got %s", result.Mode)
	}
	if result.ImageRef != fallback {
		t.Fatalf("expected fallback image %q, got %q", fallback, result.ImageRef)
	}
	if got := strings.Join(svc.pullRefs, ","); got != primary+","+fallback {
		t.Fatalf("pull refs = %q, want %q", got, primary+","+fallback)
	}
}

func TestPrepareImageForCreateSkipsExistingWorkspaceMirror(t *testing.T) {
	primary := "docker.io/memohai/workspace:debian"
	fallback := "memoh.cn/memohai/workspace:debian"
	svc := &legacyRouteTestService{
		getImageErrs: map[string]error{
			primary: ctr.ErrNotFound,
		},
	}
	m := newLegacyRouteTestManager(t, svc, config.WorkspaceConfig{
		ImagePullPolicy: config.ImagePullPolicyIfNotPresent,
	})

	result, err := m.PrepareImageForCreate(context.Background(), "memohai/workspace:debian", nil)
	if err != nil {
		t.Fatalf("PrepareImageForCreate returned error: %v", err)
	}
	if result.Mode != ImagePrepareSkipped {
		t.Fatalf("expected skipped, got %s", result.Mode)
	}
	if result.ImageRef != fallback {
		t.Fatalf("expected fallback image %q, got %q", fallback, result.ImageRef)
	}
	if svc.pullCalls != 0 {
		t.Fatalf("expected no pull calls, got %d", svc.pullCalls)
	}
	if got := strings.Join(svc.getImageRefs, ","); got != primary+","+fallback {
		t.Fatalf("get refs = %q, want %q", got, primary+","+fallback)
	}
}

func TestPrepareImageForCreateDoesNotFallbackForCustomImages(t *testing.T) {
	svc := &legacyRouteTestService{
		getImageErr: ctr.ErrNotFound,
		pullErr:     ctr.ErrRuntime,
	}
	m := newLegacyRouteTestManager(t, svc, config.WorkspaceConfig{
		ImagePullPolicy: config.ImagePullPolicyIfNotPresent,
	})

	_, err := m.PrepareImageForCreate(context.Background(), "debian:bookworm-slim", nil)
	if !errors.Is(err, ctr.ErrRuntime) {
		t.Fatalf("expected runtime error, got %v", err)
	}
	if svc.pullCalls != 1 {
		t.Fatalf("expected one pull call, got %d", svc.pullCalls)
	}
}
