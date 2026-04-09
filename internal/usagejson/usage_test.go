package usagejson

import (
    "strings"
    "testing"

    sdk "github.com/memohai/twilight-ai/sdk"
)

func TestMarshalUsesCamelCaseKeys(t *testing.T) {
    raw := string(Marshal(sdk.Usage{
        InputTokens:      12,
        OutputTokens:     34,
        TotalTokens:      46,
        ReasoningTokens:  5,
        CachedInputTokens: 6,
        InputTokenDetails: sdk.InputTokenDetail{
            NoCacheTokens: 1,
            CacheReadTokens: 2,
            CacheWriteTokens: 3,
        },
        OutputTokenDetails: sdk.OutputTokenDetail{
            TextTokens: 4,
            ReasoningTokens: 5,
        },
    }))

    for _, want := range []string{"\"inputTokens\":12", "\"outputTokens\":34", "\"cacheReadTokens\":2", "\"cacheWriteTokens\":3"} {
        if !strings.Contains(raw, want) {
            t.Fatalf("expected marshaled usage to contain %s, got %s", want, raw)
        }
    }

    for _, unwanted := range []string{"input_tokens", "output_tokens", "cache_read_tokens", "cache_write_tokens"} {
        if strings.Contains(raw, unwanted) {
            t.Fatalf("expected marshaled usage not to contain %s, got %s", unwanted, raw)
        }
    }
}
