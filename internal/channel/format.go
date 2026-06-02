package channel

import (
	"regexp"
	"strings"
)

// ContainsMarkdown returns true if the text contains common Markdown constructs.
func ContainsMarkdown(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	patterns := []string{
		`\*\*[^*]+\*\*`,
		`\*[^*]+\*`,
		`~~[^~]+~~`,
		"`[^`]+`",
		"```[\\s\\S]*```",
		`\[.+\]\(.+\)`,
		`(?m)^#{1,6}\s`,
		`(?m)^[-*]\s`,
		`(?m)^\d+\.\s`,
	}
	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			return true
		}
	}
	return false
}

// StripInlineMarkup removes the inline Markdown markers (** and `) authored for
// capable channels, leaving clean text for plain-text-only channels.
func StripInlineMarkup(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "`", "")
	return s
}

// CoerceFormatForCaps degrades msg.Format when the target channel cannot
// render it, rather than letting validateMessageCapabilities reject the
// message (which historically surfaced as a silent failure: the error was
// logged but the user saw nothing).
//
// Today only the Markdown→Plain degradation is meaningful — bullet-list
// auto-detection in normalizeOutboundMessage can wrongly promote a
// plain-by-intent body, and the channels affected (Weixin/WeChat OA/Local-Web)
// can losslessly read the body with inline markers stripped. Rich-text and
// button-bearing messages have no equivalent fallback and remain rejected by
// validation.
//
// This makes the Format=Plain invariant a property of the outbound boundary
// rather than discipline at every channel.Message{...} construction site.
func CoerceFormatForCaps(msg Message, caps ChannelCapabilities) Message {
	if msg.Format == MessageFormatMarkdown && !caps.Markdown && !caps.RichText {
		msg.Text = StripInlineMarkup(msg.Text)
		msg.Format = MessageFormatPlain
	}
	return msg
}
