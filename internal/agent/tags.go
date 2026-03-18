package agent

import (
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// TagResolver parses the inner content of a specific XML-like tag.
type TagResolver struct {
	Tag   string
	Parse func(content string) []any
}

// TagEvent is a parsed tag occurrence.
type TagEvent struct {
	Tag  string
	Data []any
}

// TagStreamResult is the output of a streaming tag extraction step.
type TagStreamResult struct {
	VisibleText string
	Events      []TagEvent
}

// DefaultTagResolvers returns the standard set of tag resolvers.
func DefaultTagResolvers() []TagResolver {
	return []TagResolver{
		AttachmentsResolver(),
		ReactionsResolver(),
		SpeechResolver(),
	}
}

// AttachmentsResolver parses <attachments> blocks into FileAttachment items.
func AttachmentsResolver() TagResolver {
	return TagResolver{
		Tag: "attachments",
		Parse: func(content string) []any {
			seen := make(map[string]struct{})
			var result []any
			for _, line := range strings.Split(content, "\n") {
				line = strings.TrimSpace(line)
				if !strings.HasPrefix(line, "-") {
					continue
				}
				path := strings.TrimSpace(line[1:])
				if path == "" {
					continue
				}
				if _, ok := seen[path]; ok {
					continue
				}
			seen[path] = struct{}{}
			att := FileAttachment{Path: path, Type: "file", Name: filenameFromPath(path)}
			if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
				att = FileAttachment{URL: path, Type: "image", Name: filenameFromURL(path)}
			}
			result = append(result, att)
			}
			return result
		},
	}
}

// ReactionsResolver parses <reactions> blocks into ReactionItem items.
func ReactionsResolver() TagResolver {
	return TagResolver{
		Tag: "reactions",
		Parse: func(content string) []any {
			seen := make(map[string]struct{})
			var result []any
			for _, line := range strings.Split(content, "\n") {
				line = strings.TrimSpace(line)
				if !strings.HasPrefix(line, "-") {
					continue
				}
				emoji := strings.TrimSpace(line[1:])
				if emoji == "" {
					continue
				}
				if _, ok := seen[emoji]; ok {
					continue
				}
				seen[emoji] = struct{}{}
				result = append(result, ReactionItem{Emoji: emoji})
			}
			return result
		},
	}
}

// SpeechResolver parses <speech> blocks into SpeechItem items.
func SpeechResolver() TagResolver {
	return TagResolver{
		Tag: "speech",
		Parse: func(content string) []any {
			text := strings.TrimSpace(content)
			if text == "" {
				return nil
			}
			return []any{SpeechItem{Text: text}}
		},
	}
}

// ExtractTagsFromText extracts and removes all tag blocks from a complete string.
func ExtractTagsFromText(text string, resolvers []TagResolver) (string, []TagEvent) {
	var events []TagEvent
	cleaned := text
	for _, r := range resolvers {
		open := "<" + r.Tag + ">"
		close := "</" + r.Tag + ">"
		pattern := regexp.MustCompile(regexp.QuoteMeta(open) + `([\s\S]*?)` + regexp.QuoteMeta(close))
		cleaned = pattern.ReplaceAllStringFunc(cleaned, func(match string) string {
			inner := match[len(open) : len(match)-len(close)]
			parsed := r.Parse(inner)
			if len(parsed) > 0 {
				events = append(events, TagEvent{Tag: r.Tag, Data: parsed})
			}
			return ""
		})
	}
	cleaned = regexp.MustCompile(`\n{3,}`).ReplaceAllString(cleaned, "\n\n")
	cleaned = strings.TrimSpace(cleaned)
	return cleaned, events
}

// StreamTagExtractor is an incremental state machine that intercepts tag blocks
// from a stream of text deltas.
type StreamTagExtractor struct {
	metas      []resolverMeta
	maxOpenLen int
	state      int // 0 = text, 1 = inside
	activeMeta *resolverMeta
	buffer     string
	tagBuffer  string
}

type resolverMeta struct {
	resolver TagResolver
	openTag  string
	closeTag string
}

// NewStreamTagExtractor creates a new streaming tag extractor.
func NewStreamTagExtractor(resolvers []TagResolver) *StreamTagExtractor {
	metas := make([]resolverMeta, len(resolvers))
	maxOpenLen := 0
	for i, r := range resolvers {
		open := "<" + r.Tag + ">"
		close := "</" + r.Tag + ">"
		metas[i] = resolverMeta{resolver: r, openTag: open, closeTag: close}
		if len(open) > maxOpenLen {
			maxOpenLen = len(open)
		}
	}
	return &StreamTagExtractor{
		metas:      metas,
		maxOpenLen: maxOpenLen,
	}
}

// safeUTF8SplitIndex adjusts a byte split index so it does not fall in the
// middle of a multi-byte UTF-8 character.  It backs up to the start of the
// rune that contains idx, guaranteeing both halves are valid UTF-8.
func safeUTF8SplitIndex(s string, idx int) int {
	if idx <= 0 || idx >= len(s) {
		return idx
	}
	for idx > 0 && !utf8.RuneStart(s[idx]) {
		idx--
	}
	return idx
}

// Push processes a text delta and returns visible text and any completed tag events.
func (e *StreamTagExtractor) Push(delta string) TagStreamResult {
	e.buffer += delta
	visible := ""
	var events []TagEvent

	for len(e.buffer) > 0 {
		if e.state == 0 { // text
			earliestIdx := -1
			var matchedMeta *resolverMeta
			for i := range e.metas {
				idx := strings.Index(e.buffer, e.metas[i].openTag)
				if idx != -1 && (earliestIdx == -1 || idx < earliestIdx) {
					earliestIdx = idx
					matchedMeta = &e.metas[i]
				}
			}
			if earliestIdx == -1 {
				keep := e.maxOpenLen - 1
				if keep > len(e.buffer) {
					keep = len(e.buffer)
				}
				splitAt := safeUTF8SplitIndex(e.buffer, len(e.buffer)-keep)
				emit := e.buffer[:splitAt]
				visible += emit
				e.buffer = e.buffer[splitAt:]
				break
			}
			visible += e.buffer[:earliestIdx]
			e.buffer = e.buffer[earliestIdx+len(matchedMeta.openTag):]
			e.tagBuffer = ""
			e.activeMeta = matchedMeta
			e.state = 1
			continue
		}

		// state == 1 (inside)
		closeTag := e.activeMeta.closeTag
		endIdx := strings.Index(e.buffer, closeTag)
		if endIdx == -1 {
			keep := len(closeTag) - 1
			if keep > len(e.buffer) {
				keep = len(e.buffer)
			}
			splitAt := safeUTF8SplitIndex(e.buffer, len(e.buffer)-keep)
			take := e.buffer[:splitAt]
			e.tagBuffer += take
			e.buffer = e.buffer[splitAt:]
			break
		}
		e.tagBuffer += e.buffer[:endIdx]
		parsed := e.activeMeta.resolver.Parse(e.tagBuffer)
		if len(parsed) > 0 {
			events = append(events, TagEvent{Tag: e.activeMeta.resolver.Tag, Data: parsed})
		}
		e.buffer = e.buffer[endIdx+len(closeTag):]
		e.tagBuffer = ""
		e.activeMeta = nil
		e.state = 0
	}

	return TagStreamResult{VisibleText: visible, Events: events}
}

// FlushRemainder flushes any remaining buffered content. Call when the stream ends.
func (e *StreamTagExtractor) FlushRemainder() TagStreamResult {
	if e.state == 0 {
		out := e.buffer
		e.buffer = ""
		return TagStreamResult{VisibleText: out}
	}
	out := e.activeMeta.openTag + e.tagBuffer + e.buffer
	e.state = 0
	e.buffer = ""
	e.tagBuffer = ""
	e.activeMeta = nil
	return TagStreamResult{VisibleText: out}
}

func filenameFromPath(p string) string {
	return filepath.Base(p)
}

func filenameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	base := filepath.Base(u.Path)
	if base == "." || base == "/" {
		return ""
	}
	return base
}
