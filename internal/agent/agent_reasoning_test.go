package agent

import (
	"context"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/models"
)

type recordingReasoningProvider struct {
	params sdk.GenerateParams
}

func (*recordingReasoningProvider) Name() string {
	return "openai-completions"
}

func (*recordingReasoningProvider) ListModels(context.Context) ([]sdk.Model, error) {
	return nil, nil
}

func (*recordingReasoningProvider) Test(context.Context) *sdk.ProviderTestResult {
	return &sdk.ProviderTestResult{Status: sdk.ProviderStatusOK}
}

func (*recordingReasoningProvider) TestModel(context.Context, string) (*sdk.ModelTestResult, error) {
	return &sdk.ModelTestResult{Supported: true}, nil
}

func (p *recordingReasoningProvider) DoGenerate(_ context.Context, params sdk.GenerateParams) (*sdk.GenerateResult, error) {
	p.params = params
	return &sdk.GenerateResult{
		Text:         "ok",
		FinishReason: sdk.FinishReasonStop,
	}, nil
}

func (*recordingReasoningProvider) DoStream(context.Context, sdk.GenerateParams) (*sdk.StreamResult, error) {
	return nil, nil
}

func TestBuildGenerateOptionsPreservesDeepSeekReasoningDisabled(t *testing.T) {
	t.Parallel()

	provider := &recordingReasoningProvider{}
	cfg := RunConfig{
		Model: &sdk.Model{
			ID:       "deepseek-v4-flash",
			Provider: provider,
			Type:     sdk.ModelTypeChat,
		},
		ReasoningDisabled:     true,
		ChatCompletionsCompat: models.ChatCompletionsCompatDeepSeek,
	}

	opts := (*Agent)(nil).buildGenerateOptions(cfg, nil, nil, nil)
	if _, err := sdk.GenerateTextResult(context.Background(), opts...); err != nil {
		t.Fatalf("generate text result: %v", err)
	}
	if provider.params.ReasoningEffort == nil {
		t.Fatal("expected reasoning effort to be set")
	}
	if got := *provider.params.ReasoningEffort; got != "none" {
		t.Fatalf("expected reasoning effort none, got %q", got)
	}
}

func TestIsAskUserArgumentParseError(t *testing.T) {
	t.Parallel()

	if !isAskUserArgumentParseError(`openai: unmarshal tool call arguments for "ask_user": invalid character 'ç' after object key:value pair`) {
		t.Fatal("expected ask_user argument parse error to match")
	}
	if isAskUserArgumentParseError(`openai: unmarshal tool call arguments for "web_search": invalid character`) {
		t.Fatal("expected other tool argument errors not to match")
	}
}
