package command

import (
	"errors"
	"strings"
)

// ParsedCommand holds the parsed components of a slash command.
type ParsedCommand struct {
	Resource string   // e.g. "schedule", "subagent", "help"
	Action   string   // e.g. "list", "get", "create"
	Args     []string // remaining positional arguments
}

// Parse parses a raw command string into its components.
// Expected format: /resource [action] [args...].
func Parse(text string) (ParsedCommand, error) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return ParsedCommand{}, errors.New("command must start with /")
	}
	text = text[1:] // strip leading /

	tokens := tokenize(text)
	if len(tokens) == 0 {
		return ParsedCommand{}, errors.New("empty command")
	}

	resource := strings.ToLower(tokens[0])
	// Strip Telegram-style @botname suffix (e.g. "help@MemohBot" -> "help").
	if idx := strings.IndexByte(resource, '@'); idx > 0 {
		resource = resource[:idx]
	}

	cmd := ParsedCommand{
		Resource: resource,
	}
	if len(tokens) > 1 {
		cmd.Action = strings.ToLower(tokens[1])
	}
	if len(tokens) > 2 {
		cmd.Args = tokens[2:]
	}
	return cmd, nil
}

// ExtractCommandText finds and extracts a slash command from text that may
// contain a leading @mention (e.g. "@BotName /help arg1" -> "/help arg1").
// Returns the command text starting with "/", or empty string if none found.
func ExtractCommandText(text string) string {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "/") {
		return trimmed
	}
	// Look for " /" pattern — a slash preceded by whitespace.
	idx := strings.Index(trimmed, " /")
	if idx >= 0 {
		return strings.TrimSpace(trimmed[idx+1:])
	}
	return ""
}

// tokenize splits a command string respecting quoted segments.
func tokenize(input string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	for i := 0; i < len(input); i++ {
		ch := input[i]
		if inQuote {
			if ch == quoteChar {
				inQuote = false
				continue
			}
			current.WriteByte(ch)
			continue
		}
		if ch == '"' || ch == '\'' {
			inQuote = true
			quoteChar = ch
			continue
		}
		if ch == ' ' || ch == '\t' {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteByte(ch)
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}
