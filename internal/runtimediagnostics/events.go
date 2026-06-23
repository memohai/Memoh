package runtimediagnostics

import (
	"regexp"
	"strings"
)

const redactedValue = "[redacted]"

var (
	sensitiveAssignmentPattern = regexp.MustCompile(`(?i)(["']?\b(?:api[_-]?key|apikey|oauth[_-]?token|access[_-]?token|refresh[_-]?token|id[_-]?token|auth[_-]?token|client[_-]?secret|password|passwd|secret|authorization|token)\b["']?\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s,;}]+)`)
	bearerTokenPattern         = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`)
)

func SanitizeEventMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		out[key] = sanitizeMetadataValue(key, value)
	}
	return out
}

func SanitizeEventMessage(message string) string {
	message = bearerTokenPattern.ReplaceAllString(message, "Bearer "+redactedValue)
	return sensitiveAssignmentPattern.ReplaceAllString(message, `$1`+redactedValue)
}

func sanitizeMetadataValue(key string, value any) any {
	if isSensitiveMetadataKey(key) {
		return redactedValue
	}
	switch typed := value.(type) {
	case map[string]any:
		return SanitizeEventMetadata(typed)
	case map[string]string:
		out := make(map[string]any, len(typed))
		for k, v := range typed {
			out[k] = sanitizeMetadataValue(k, v)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = sanitizeMetadataValue("", item)
		}
		return out
	case []string:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = SanitizeEventMessage(item)
		}
		return out
	case string:
		return SanitizeEventMessage(typed)
	default:
		return value
	}
}

func isSensitiveMetadataKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if normalized == "" {
		return false
	}
	switch normalized {
	case "api_key", "apikey", "key", "password", "passwd", "secret", "client_secret",
		"token", "access_token", "refresh_token", "id_token", "oauth_token", "auth_token",
		"authorization", "cookie", "set_cookie":
		return true
	}
	return strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password")
}
