package usagejson

import (
    "encoding/json"

    sdk "github.com/memohai/twilight-ai/sdk"
)

type Usage struct {
    InputTokens        int                `json:"inputTokens,omitempty"`
    OutputTokens       int                `json:"outputTokens,omitempty"`
    TotalTokens        int                `json:"totalTokens,omitempty"`
    ReasoningTokens    int                `json:"reasoningTokens,omitempty"`
    CachedInputTokens  int                `json:"cachedInputTokens,omitempty"`
    InputTokenDetails  InputTokenDetails  `json:"inputTokenDetails,omitempty"`
    OutputTokenDetails OutputTokenDetails `json:"outputTokenDetails,omitempty"`
}

type InputTokenDetails struct {
    NoCacheTokens    int `json:"noCacheTokens,omitempty"`
    CacheReadTokens  int `json:"cacheReadTokens,omitempty"`
    CacheWriteTokens int `json:"cacheWriteTokens,omitempty"`
}

type OutputTokenDetails struct {
    TextTokens      int `json:"textTokens,omitempty"`
    ReasoningTokens int `json:"reasoningTokens,omitempty"`
}

func FromSDK(u sdk.Usage) Usage {
    return Usage{
        InputTokens:       u.InputTokens,
        OutputTokens:      u.OutputTokens,
        TotalTokens:       u.TotalTokens,
        ReasoningTokens:   u.ReasoningTokens,
        CachedInputTokens: u.CachedInputTokens,
        InputTokenDetails: InputTokenDetails{
            NoCacheTokens:    u.InputTokenDetails.NoCacheTokens,
            CacheReadTokens:  u.InputTokenDetails.CacheReadTokens,
            CacheWriteTokens: u.InputTokenDetails.CacheWriteTokens,
        },
        OutputTokenDetails: OutputTokenDetails{
            TextTokens:      u.OutputTokenDetails.TextTokens,
            ReasoningTokens: u.OutputTokenDetails.ReasoningTokens,
        },
    }
}

func Marshal(u sdk.Usage) []byte {
    b, _ := json.Marshal(FromSDK(u))
    return b
}

func MarshalPtr(u *sdk.Usage) []byte {
    if u == nil {
        return nil
    }
    return Marshal(*u)
}
