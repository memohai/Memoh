package command

import (
	"errors"
	"strconv"
	"strings"
)

// ParsedCommand holds the parsed components of a slash command.
type ParsedCommand struct {
	Resource string   // e.g. "schedule", "subagent", "help"
	Action   string   // e.g. "list", "get", "create"
	Args     []string // remaining positional arguments
	Page     int      // zero-based page offset from a "--page N" flag (0 if absent)
	Prov     int      // provider index from a "--prov N" flag (-1 if absent)
	SelectID string   // stable model id from a "--id V" flag ("" if absent)
	Range    string   // time-window key from a "--range V" flag ("" if absent)
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
	tokens, flags := extractFlags(tokens)
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
		Page:     flags.page,
		Prov:     flags.prov,
		SelectID: flags.selectID,
		Range:    flags.rangeKey,
	}
	if len(tokens) > 1 {
		cmd.Action = strings.ToLower(tokens[1])
	}
	if len(tokens) > 2 {
		cmd.Args = tokens[2:]
	}
	return cmd, nil
}

type parsedFlags struct {
	page     int
	prov     int
	rangeKey string
	selectID string
}

// extractFlags pulls "--page N", "--prov N" (ints), "--range V" and "--id V"
// (strings) out of the token stream so they do not leak into positional Args.
// Absent integer flags default to 0 for page and -1 for prov; strings to "".
func extractFlags(tokens []string) ([]string, parsedFlags) {
	out := make([]string, 0, len(tokens))
	flags := parsedFlags{page: 0, prov: -1}
	for i := 0; i < len(tokens); i++ {
		if tokens[i] == "--range" && i+1 < len(tokens) {
			flags.rangeKey = tokens[i+1]
			i++ // skip the value token
			continue
		}
		if tokens[i] == "--id" && i+1 < len(tokens) {
			flags.selectID = tokens[i+1]
			i++ // skip the value token
			continue
		}
		var target *int
		minVal := 0
		switch tokens[i] {
		case "--page":
			target = &flags.page
			minVal = 0
		case "--prov":
			target = &flags.prov
			minVal = 0
		}
		if target != nil && i+1 < len(tokens) {
			if n, err := strconv.Atoi(tokens[i+1]); err == nil && n >= minVal {
				*target = n
				i++ // skip the value token
				continue
			}
		}
		out = append(out, tokens[i])
	}
	return out, flags
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
