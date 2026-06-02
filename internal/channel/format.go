package channel

import (
	"regexp"
	"strings"
)

// markdownPatterns lists the constructs that flag a string as Markdown. Compiled
// once at package init so the per-call ContainsMarkdown scan is a fixed-cost
// iteration over precompiled patterns instead of recompiling every regex on
// every call — important because ContainsMarkdown sits on hot paths (every
// streaming delta, every outbound normalization).
var markdownPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\*\*[^*]+\*\*`),
	regexp.MustCompile(`\*[^*]+\*`),
	regexp.MustCompile(`~~[^~]+~~`),
	regexp.MustCompile("`[^`]+`"),
	regexp.MustCompile("```[\\s\\S]*```"),
	regexp.MustCompile(`\[.+\]\(.+\)`),
	regexp.MustCompile(`(?m)^#{1,6}\s`),
	regexp.MustCompile(`(?m)^[-*]\s`),
	regexp.MustCompile(`(?m)^\d+\.\s`),
}

// ContainsMarkdown returns true if the text contains common Markdown constructs.
func ContainsMarkdown(text string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	for _, p := range markdownPatterns {
		if p.MatchString(text) {
			return true
		}
	}
	return false
}

// StripInlineMarkup removes the inline Markdown markers (** and `) authored for
// capable channels, leaving clean text for plain-text-only channels.
//
// Scope: only ** (bold) and ` (code) are stripped, because those are the only
// inline markers the command renderers emit (MdBold/MdCode/CmdRef). Other
// constructs ContainsMarkdown recognizes — links [a](b), headings, list
// bullets — are intentionally NOT stripped: the renderers never produce them,
// so any such characters in a body are literal user/content text and must be
// preserved verbatim rather than mangled. Extend this (and coerceFormatForCaps)
// if a renderer ever starts emitting those constructs.
func StripInlineMarkup(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "`", "")
	return s
}

// coerceFormatForCaps degrades msg.Format when the target channel cannot
// render it. Called right before validateMessageCapabilities at the outbound
// boundary so a Markdown-typed body destined for a plain-text-only channel
// gets stripped + retyped instead of being rejected.
//
// Today only Markdown→Plain is lossless enough to degrade automatically
// (strip bold and code markers, retype). Rich-format bodies (with Parts) and
// button-bearing bodies have no equivalent fallback and remain rejected by
// validation — extend this function (and its tests) when a handler emits
// such a body on a non-capable channel.
func coerceFormatForCaps(msg Message, caps ChannelCapabilities) Message {
	if msg.Format == MessageFormatMarkdown && !caps.Markdown && !caps.RichText {
		msg.Text = StripInlineMarkup(msg.Text)
		msg.Format = MessageFormatPlain
	}
	return msg
}
