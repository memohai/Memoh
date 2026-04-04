package flow

import (
	"context"
	"strings"
	"testing"

	agentpkg "github.com/memohai/memoh/internal/agent"
)

const imageReadHint = "also supports images: PNG, JPEG, GIF, WebP"

func TestPrepareRunConfigIncludesImageReadHintWhenImageInputIsSupported(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{}
	cfg := agentpkg.RunConfig{
		Query:              "describe this image",
		SupportsImageInput: true,
		Identity: agentpkg.SessionContext{
			BotID: "bot-1",
		},
	}

	prepared := resolver.prepareRunConfig(context.Background(), cfg)
	if !strings.Contains(prepared.System, imageReadHint) {
		t.Fatalf("expected system prompt to contain %q, got:\n%s", imageReadHint, prepared.System)
	}
}

func TestPrepareRunConfigOmitsImageReadHintWhenImageInputIsUnsupported(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{}
	cfg := agentpkg.RunConfig{
		Query:              "describe this image",
		SupportsImageInput: false,
		Identity: agentpkg.SessionContext{
			BotID: "bot-1",
		},
	}

	prepared := resolver.prepareRunConfig(context.Background(), cfg)
	if strings.Contains(prepared.System, imageReadHint) {
		t.Fatalf("expected system prompt to NOT contain %q, got:\n%s", imageReadHint, prepared.System)
	}
}
