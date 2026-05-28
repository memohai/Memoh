package models

import (
	"strings"
)

// ChatCompletionsCompatDeepSeek enables DeepSeek request compatibility while
// still using the generic OpenAI Chat Completions provider.
const ChatCompletionsCompatDeepSeek = "deepseek"

func normalizeChatCompletionsCompat(compat string) string {
	return strings.ToLower(strings.TrimSpace(compat))
}

func isDeepSeekChatCompletionsCompat(compat string) bool {
	return normalizeChatCompletionsCompat(compat) == ChatCompletionsCompatDeepSeek
}

// ResolveChatCompletionsCompat returns a normalized compatibility mode from
// explicit config, with a fallback for the built-in DeepSeek endpoint.
func ResolveChatCompletionsCompat(baseURL, compat string) string {
	compat = normalizeChatCompletionsCompat(compat)
	if compat != "" {
		return compat
	}
	if strings.Contains(strings.ToLower(strings.TrimSpace(baseURL)), "api.deepseek.com") {
		return ChatCompletionsCompatDeepSeek
	}
	return ""
}
