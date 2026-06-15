package discord

import "strings"

// discordEscapeLinkText escapes the characters that can prematurely close or
// split a `[text](url)` label, and collapses control whitespace.
func discordEscapeLinkText(text string) string {
	text = strings.ReplaceAll(text, "\\", "\\\\")
	text = strings.ReplaceAll(text, "]", "\\]")
	return text
}

// discordEscapeLinkURL strips characters that would prematurely terminate a
// markdown link URL, then percent-encodes the few that Discord still parses
// inside `[label](url)`.
func discordEscapeLinkURL(url string) string {
	url = strings.ReplaceAll(strings.TrimSpace(url), "\n", "")
	url = strings.ReplaceAll(url, "\r", "")
	url = strings.ReplaceAll(url, ")", "%29")
	return url
}
