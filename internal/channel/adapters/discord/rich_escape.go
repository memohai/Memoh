package discord

import "strings"

// discordEscapeLinkURL strips characters that would prematurely terminate a
// markdown link URL, then percent-encodes the few that Discord still parses
// inside `[label](url)`.
func discordEscapeLinkURL(url string) string {
	url = strings.ReplaceAll(strings.TrimSpace(url), "\n", "")
	url = strings.ReplaceAll(url, "\r", "")
	url = strings.ReplaceAll(url, " ", "%20")
	url = strings.ReplaceAll(url, "<", "%3C")
	url = strings.ReplaceAll(url, ">", "%3E")
	url = strings.ReplaceAll(url, "(", "%28")
	url = strings.ReplaceAll(url, ")", "%29")
	return url
}
