package slash

import "strings"

type Surface string

const (
	SurfaceChannel Surface = "channel"
	SurfaceWebWS   Surface = "web_ws"
	SurfaceWebREST Surface = "web_rest"
)

type DecisionKind string

const (
	DecisionNormalChat         DecisionKind = "normal_chat"
	DecisionRejectNoop         DecisionKind = "reject_noop"
	DecisionCommandAction      DecisionKind = "command_action"
	DecisionUnsupportedCommand DecisionKind = "unsupported_command"
	DecisionSkillIntent        DecisionKind = "skill_intent"
	DecisionUnknownSlash       DecisionKind = "unknown_slash"
	DecisionReject             DecisionKind = "reject"
)

type Command struct {
	Resource string
	Action   string
	Raw      string
}

type SkillIntent struct {
	Names  []string
	Prompt string
}

type Decision struct {
	Kind        DecisionKind
	Code        string
	Directed    bool
	Command     Command
	SkillIntent SkillIntent
}

type ClassifyInput struct {
	Text               string
	HasAttachments     bool
	Surface            Surface
	IsGroup            bool
	Directed           bool
	SupportsMode       bool
	BotAliases         []string
	KnownCommand       func(resource string) bool
	WebActionSupported func(resource, action string) bool
}

func Classify(input ClassifyInput) Decision {
	text := strings.TrimSpace(input.Text)
	if text == "" || !isSlashLike(text, input.BotAliases) {
		return Decision{Kind: DecisionNormalChat, Directed: input.Directed}
	}

	cmdText, directedByText := extractSlashText(text, input.BotAliases)
	effectiveDirected := input.Directed || directedByText || slashCommandSuffixMatches(cmdText, input.BotAliases)
	if input.Surface == SurfaceChannel && input.IsGroup && !effectiveDirected {
		return Decision{Kind: DecisionRejectNoop, Directed: false}
	}
	if input.HasAttachments {
		return Decision{Kind: DecisionReject, Code: CodeSlashAttachmentsUnsupported, Directed: effectiveDirected}
	}

	parsed, ok := parseCommand(cmdText, input.BotAliases)
	if !ok {
		return Decision{Kind: DecisionUnknownSlash, Code: CodeUnknownSlash, Directed: effectiveDirected}
	}
	effectiveDirected = effectiveDirected || parsed.Directed

	if parsed.Resource == "skill" && parsed.Action == "use" {
		if input.Surface == SurfaceWebWS || input.Surface == SurfaceWebREST {
			return Decision{Kind: DecisionReject, Code: CodeUseSkillChipRequired, Directed: effectiveDirected}
		}
		intent, code := parseSkillUse(parsed.Args)
		if code != "" {
			return Decision{Kind: DecisionReject, Code: code, Directed: effectiveDirected}
		}
		return Decision{Kind: DecisionSkillIntent, Directed: effectiveDirected, Command: parsed.Command(), SkillIntent: intent}
	}

	if input.Surface == SurfaceChannel && input.SupportsMode && isModePrefix(parsed.Resource) {
		remainder := strings.TrimSpace(parsed.Rest)
		if strings.HasPrefix(remainder, "/") || isSlashLike(remainder, input.BotAliases) {
			return Decision{Kind: DecisionReject, Code: CodeUnknownSlash, Directed: effectiveDirected}
		}
		return Decision{Kind: DecisionNormalChat, Directed: effectiveDirected}
	}

	if isKnown(input.KnownCommand, parsed.Resource) {
		cmd := parsed.Command()
		if input.Surface == SurfaceWebWS || input.Surface == SurfaceWebREST {
			if input.WebActionSupported != nil && input.WebActionSupported(parsed.Resource, parsed.Action) {
				return Decision{Kind: DecisionCommandAction, Directed: effectiveDirected, Command: cmd}
			}
			return Decision{Kind: DecisionUnsupportedCommand, Code: CodeUnsupportedWebCommand, Directed: effectiveDirected, Command: cmd}
		}
		return Decision{Kind: DecisionCommandAction, Directed: effectiveDirected, Command: cmd}
	}

	return Decision{Kind: DecisionUnknownSlash, Code: CodeUnknownSlash, Directed: effectiveDirected}
}

type parsedCommand struct {
	Resource string
	Action   string
	Args     string
	Rest     string
	Raw      string
	Directed bool
}

func (p parsedCommand) Command() Command {
	return Command{Resource: p.Resource, Action: p.Action, Raw: p.Raw}
}

func isSlashLike(text string, aliases []string) bool {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "/") {
		return true
	}
	if stripped, ok := stripLeadingMention(text, aliases); ok {
		return strings.HasPrefix(strings.TrimSpace(stripped), "/")
	}
	return false
}

func extractSlashText(text string, aliases []string) (string, bool) {
	if stripped, ok := stripLeadingMention(text, aliases); ok {
		return strings.TrimSpace(stripped), true
	}
	return strings.TrimSpace(text), false
}

func slashCommandSuffixMatches(text string, aliases []string) bool {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return false
	}
	head := strings.TrimPrefix(text, "/")
	if idx := strings.IndexAny(head, " \t\r\n"); idx >= 0 {
		head = head[:idx]
	}
	if before, after, ok := strings.Cut(head, "@"); ok && before != "" {
		return aliasMatches(after, aliases)
	}
	return false
}

func stripLeadingMention(text string, aliases []string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) < 2 {
		return "", false
	}
	first := strings.TrimPrefix(fields[0], "@")
	if !aliasMatches(first, aliases) {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), fields[0])), true
}

func parseCommand(text string, aliases []string) (parsedCommand, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/") {
		return parsedCommand{}, false
	}
	raw := text
	text = strings.TrimPrefix(text, "/")
	head, rest, _ := strings.Cut(text, " ")
	head = strings.TrimSpace(head)
	rest = strings.TrimSpace(rest)
	if head == "" {
		return parsedCommand{}, false
	}

	resource := head
	directed := false
	if before, after, ok := strings.Cut(head, "@"); ok {
		if !aliasMatches(after, aliases) {
			return parsedCommand{}, false
		}
		resource = before
		directed = true
	}
	if !isCommandName(resource) {
		return parsedCommand{}, false
	}

	action := ""
	args := ""
	if rest != "" {
		action, args, _ = strings.Cut(rest, " ")
		action = strings.TrimSpace(action)
		args = strings.TrimSpace(args)
	}
	return parsedCommand{
		Resource: strings.ToLower(resource),
		Action:   strings.ToLower(action),
		Args:     args,
		Rest:     rest,
		Raw:      raw,
		Directed: directed,
	}, true
}

func isKnown(fn func(string) bool, resource string) bool {
	return fn != nil && fn(resource)
}

func isModePrefix(resource string) bool {
	switch resource {
	case "now", "btw", "next":
		return true
	default:
		return false
	}
}

func parseSkillUse(args string) (SkillIntent, string) {
	selector, prompt, ok := splitSkillUseSelectorPrompt(args)
	if !ok {
		return SkillIntent{}, CodeInvalidSkillSlashSyntax
	}
	selector = strings.TrimSpace(selector)
	prompt = strings.TrimSpace(prompt)
	if selector == "" {
		return SkillIntent{}, CodeInvalidSkillSlashSyntax
	}
	if prompt == "" {
		return SkillIntent{}, CodeMissingPrompt
	}
	parts := strings.Split(selector, ",")
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if !isValidSkillSelector(name) {
			return SkillIntent{}, CodeInvalidSkillSlashSyntax
		}
		names = append(names, name)
	}
	return SkillIntent{Names: names, Prompt: prompt}, ""
}

func splitSkillUseSelectorPrompt(args string) (string, string, bool) {
	for i := 0; i+1 < len(args); i++ {
		if args[i] != '-' || args[i+1] != '-' {
			continue
		}
		if i == 0 || !isASCIISpace(args[i-1]) {
			continue
		}
		if i+2 < len(args) && !isASCIISpace(args[i+2]) {
			continue
		}
		return args[:i], args[i+2:], true
	}
	return "", "", false
}

func isASCIISpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r':
		return true
	default:
		return false
	}
}

func isCommandName(name string) bool {
	if name == "" || len(name) > 32 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case i > 0 && (c >= '0' && c <= '9' || c == '_' || c == '-'):
		default:
			return false
		}
	}
	return true
}

func isValidSkillSelector(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	if strings.HasPrefix(name, ".") || strings.Contains(name, "..") {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '-' || c == '_' || c == '.':
		default:
			return false
		}
	}
	return true
}

func aliasMatches(value string, aliases []string) bool {
	value = strings.Trim(strings.TrimSpace(value), "@")
	if value == "" {
		return false
	}
	for _, alias := range aliases {
		alias = strings.Trim(strings.TrimSpace(alias), "@")
		if alias != "" && strings.EqualFold(value, alias) {
			return true
		}
	}
	return false
}
