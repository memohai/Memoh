package flow

import (
	"context"
	"strings"
	"testing"

	agentpkg "github.com/memohai/memoh/internal/agent"
)

func TestPrepareRunConfigIncludesReadMediaWhenImageInputIsSupported(t *testing.T) {
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
	if !strings.Contains(prepared.System, "`read_media`") {
		t.Fatalf("expected system prompt to include read_media tool, got:\n%s", prepared.System)
	}
}

func TestPrepareRunConfigOmitsReadMediaWhenImageInputIsUnsupported(t *testing.T) {
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
	if strings.Contains(prepared.System, "`read_media`") {
		t.Fatalf("expected system prompt to omit read_media tool, got:\n%s", prepared.System)
	}
}
