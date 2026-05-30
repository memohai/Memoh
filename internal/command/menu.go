package command

// MenuCommand is one entry for a channel's native slash-command menu (e.g.
// Telegram's setMyCommands), so users discover and tap commands without typing.
type MenuCommand struct {
	Command     string // command name without the leading slash, lowercase
	Description string // short label shown beside the command in the menu
}

// MenuCommands returns the curated slash-command list to advertise in a
// channel's native command menu. It is the single source for that menu; order
// roughly follows everyday usefulness. Only single-token commands belong here —
// the native menu cannot express sub-actions like "schedule list" (those are
// discovered via /help or in-message buttons).
func MenuCommands() []MenuCommand {
	return []MenuCommand{
		{"help", "Show available commands"},
		{"new", "Start a new conversation"},
		{"stop", "Stop the current reply"},
		{"status", "Show session status"},
		{"context", "Show context window usage"},
		{"model", "Switch the chat model"},
		{"reasoning", "Set reasoning level (off/low/medium/high)"},
		{"settings", "View and update bot settings"},
		{"memory", "Choose the memory provider"},
		{"search", "Choose the search provider"},
		{"schedule", "Manage scheduled tasks"},
		{"mcp", "Manage MCP connections"},
		{"usage", "View token usage"},
		{"email", "View email configuration"},
		{"heartbeat", "View heartbeat logs"},
		{"skill", "View bot skills"},
		{"fs", "Browse container files"},
		{"access", "Inspect identity and permissions"},
		{"compact", "Compact conversation context"},
	}
}
