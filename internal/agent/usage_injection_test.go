package agent

import (
	"context"
	"strings"
	"sync"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent/tools"
)

// usageTestProvider is a ToolProvider that also implements tools.ToolUsage. It
// returns a tool (and therefore contributes usage) only when emitTool is true,
// letting us assert that usage guidance is gated by tool registration.
type usageTestProvider struct {
	emitTool      bool
	usage         string
	requireTool   tools.ToolName
	missingMarker string
}

func (p *usageTestProvider) Tools(_ context.Context, _ tools.SessionContext) ([]sdk.Tool, error) {
	if !p.emitTool {
		return nil, nil
	}
	return []sdk.Tool{{Name: "fake_tool", Description: "fake"}}, nil
}

func (p *usageTestProvider) Usage(_ context.Context, _ tools.SessionContext, available tools.AvailableTools) string {
	if p.requireTool != "" {
		if available.Has(p.requireTool) {
			return p.usage
		}
		return p.missingMarker
	}
	return p.usage
}

// plainTestProvider returns a tool but does NOT implement tools.ToolUsage.
type plainTestProvider struct{}

func (plainTestProvider) Tools(_ context.Context, _ tools.SessionContext) ([]sdk.Tool, error) {
	return []sdk.Tool{{Name: tools.ToolRead.String(), Description: "plain"}}, nil
}

func newTestAgent(providers ...tools.ToolProvider) *Agent {
	a := New(Deps{})
	a.SetToolProviders(providers)
	return a
}

const usageMarker = "USAGE_MARKER_xyz"

type usageRecordingProvider struct {
	mu     sync.Mutex
	params []sdk.GenerateParams
}

func (*usageRecordingProvider) Name() string { return "usage-recording" }

func (*usageRecordingProvider) ListModels(context.Context) ([]sdk.Model, error) { return nil, nil }

func (*usageRecordingProvider) Test(context.Context) *sdk.ProviderTestResult {
	return &sdk.ProviderTestResult{Status: sdk.ProviderStatusOK}
}

func (*usageRecordingProvider) TestModel(context.Context, string) (*sdk.ModelTestResult, error) {
	return &sdk.ModelTestResult{Supported: true}, nil
}

func (p *usageRecordingProvider) DoGenerate(_ context.Context, params sdk.GenerateParams) (*sdk.GenerateResult, error) {
	p.mu.Lock()
	p.params = append(p.params, params)
	p.mu.Unlock()
	return &sdk.GenerateResult{Text: "ok", FinishReason: sdk.FinishReasonStop}, nil
}

func (*usageRecordingProvider) DoStream(context.Context, sdk.GenerateParams) (*sdk.StreamResult, error) {
	return nil, nil
}

func (p *usageRecordingProvider) lastParams() sdk.GenerateParams {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.params) == 0 {
		return sdk.GenerateParams{}
	}
	return p.params[len(p.params)-1]
}

func TestGenerateInjectsToolUsageIntoModelSystem(t *testing.T) {
	t.Parallel()
	modelProvider := &usageRecordingProvider{}
	a := newTestAgent(&usageTestProvider{emitTool: true, usage: usageMarker})

	if _, err := a.Generate(context.Background(), RunConfig{
		Model: &sdk.Model{
			ID:       "usage-model",
			Provider: modelProvider,
			Type:     sdk.ModelTypeChat,
		},
		System:           "base system",
		Messages:         []sdk.Message{sdk.UserMessage("hi")},
		SupportsToolCall: true,
	}); err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	params := modelProvider.lastParams()
	if !strings.Contains(params.System, "## Tool usage") || !strings.Contains(params.System, usageMarker) {
		t.Fatalf("expected model system to contain injected usage, got %q", params.System)
	}
}

func TestGenerateOmitsToolUsageWhenToolCallingUnsupported(t *testing.T) {
	t.Parallel()
	modelProvider := &usageRecordingProvider{}
	a := newTestAgent(&usageTestProvider{emitTool: true, usage: usageMarker})

	if _, err := a.Generate(context.Background(), RunConfig{
		Model: &sdk.Model{
			ID:       "usage-model",
			Provider: modelProvider,
			Type:     sdk.ModelTypeChat,
		},
		System:           "base system",
		Messages:         []sdk.Message{sdk.UserMessage("hi")},
		SupportsToolCall: false,
	}); err != nil {
		t.Fatalf("Generate error: %v", err)
	}

	params := modelProvider.lastParams()
	if strings.Contains(params.System, "## Tool usage") || strings.Contains(params.System, usageMarker) {
		t.Fatalf("expected model system to omit tool usage when tools are unsupported, got %q", params.System)
	}
}

func TestAssembleToolsInjectsUsageWhenProviderEmitsTools(t *testing.T) {
	t.Parallel()
	a := newTestAgent(&usageTestProvider{emitTool: true, usage: usageMarker})

	gotTools, usage, err := a.assembleTools(context.Background(), RunConfig{}, tools.StreamEmitter(func(tools.ToolStreamEvent) {}))
	if err != nil {
		t.Fatalf("assembleTools error: %v", err)
	}
	if len(gotTools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(gotTools))
	}
	if !strings.Contains(usage, usageMarker) {
		t.Fatalf("expected usage to contain %q, got %q", usageMarker, usage)
	}
	if !strings.Contains(usage, "## Tool usage") {
		t.Fatalf("expected usage to carry the section header, got %q", usage)
	}
}

func TestAssembleToolsOmitsUsageWhenProviderEmitsNoTools(t *testing.T) {
	t.Parallel()
	a := newTestAgent(&usageTestProvider{emitTool: false, usage: usageMarker})

	gotTools, usage, err := a.assembleTools(context.Background(), RunConfig{}, tools.StreamEmitter(func(tools.ToolStreamEvent) {}))
	if err != nil {
		t.Fatalf("assembleTools error: %v", err)
	}
	if len(gotTools) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(gotTools))
	}
	if usage != "" {
		t.Fatalf("expected no usage when the provider registers no tools, got %q", usage)
	}
}

func TestAssembleToolsIgnoresProvidersWithoutUsage(t *testing.T) {
	t.Parallel()
	a := newTestAgent(plainTestProvider{})

	gotTools, usage, err := a.assembleTools(context.Background(), RunConfig{}, tools.StreamEmitter(func(tools.ToolStreamEvent) {}))
	if err != nil {
		t.Fatalf("assembleTools error: %v", err)
	}
	if len(gotTools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(gotTools))
	}
	if usage != "" {
		t.Fatalf("expected no usage from a provider that does not implement ToolUsage, got %q", usage)
	}
}

// TestAssembleToolsGatesUsagePerProvider proves the gating is per-provider, not
// global: with a mix of providers in one session, only the usage of providers
// that themselves registered tools appears. The no-tool provider's usage must
// be dropped even though another provider did contribute tools.
func TestAssembleToolsGatesUsagePerProvider(t *testing.T) {
	t.Parallel()
	const (
		markerActive = "USAGE_ACTIVE_aaa"
		markerSilent = "USAGE_SILENT_bbb"
	)
	a := newTestAgent(
		&usageTestProvider{emitTool: true, usage: markerActive},
		&usageTestProvider{emitTool: false, usage: markerSilent},
		plainTestProvider{},
	)

	gotTools, usage, err := a.assembleTools(context.Background(), RunConfig{}, tools.StreamEmitter(func(tools.ToolStreamEvent) {}))
	if err != nil {
		t.Fatalf("assembleTools error: %v", err)
	}
	if len(gotTools) != 2 {
		t.Fatalf("expected 2 tools (active usage provider + plain provider), got %d", len(gotTools))
	}
	if !strings.Contains(usage, markerActive) {
		t.Fatalf("expected usage from the tool-emitting provider, got %q", usage)
	}
	if strings.Contains(usage, markerSilent) {
		t.Fatalf("usage from a provider that registered no tools must be dropped, got %q", usage)
	}
}

func TestAssembleToolsPassesCompleteAvailableToolSetToUsage(t *testing.T) {
	t.Parallel()
	const (
		markerPresent = "USAGE_DEPENDENCY_PRESENT"
		markerMissing = "USAGE_DEPENDENCY_MISSING"
	)
	a := newTestAgent(
		&usageTestProvider{
			emitTool:      true,
			usage:         markerPresent,
			requireTool:   tools.ToolRead,
			missingMarker: markerMissing,
		},
		plainTestProvider{},
	)

	gotTools, usage, err := a.assembleTools(context.Background(), RunConfig{}, tools.StreamEmitter(func(tools.ToolStreamEvent) {}))
	if err != nil {
		t.Fatalf("assembleTools error: %v", err)
	}
	if len(gotTools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(gotTools))
	}
	if !strings.Contains(usage, markerPresent) {
		t.Fatalf("expected usage to see tool from another provider, got %q", usage)
	}
	if strings.Contains(usage, markerMissing) {
		t.Fatalf("expected complete available tool set, got missing marker in %q", usage)
	}
}
