package slack

import "strings"

// slackEscapeMrkdwn escapes characters that Slack interprets specially when
// emitting mrkdwn (`&`, `<`, `>`).
func slackEscapeMrkdwn(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}
