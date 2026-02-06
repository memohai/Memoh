package channel

import "strings"

type ChunkerMode string

const (
	ChunkerModeText     ChunkerMode = "text"
	ChunkerModeMarkdown ChunkerMode = "markdown"
)

type OutboundOrder string

const (
	OutboundOrderMediaFirst OutboundOrder = "media_first"
	OutboundOrderTextFirst  OutboundOrder = "text_first"
)

type Chunker func(text string, limit int) []string

type OutboundPolicy struct {
	TextChunkLimit int           `json:"text_chunk_limit,omitempty"`
	ChunkerMode    ChunkerMode   `json:"chunker_mode,omitempty"`
	Chunker        Chunker       `json:"-"`
	MediaOrder     OutboundOrder `json:"media_order,omitempty"`
	RetryMax       int           `json:"retry_max,omitempty"`
	RetryBackoffMs int           `json:"retry_backoff_ms,omitempty"`
}

func NormalizeOutboundPolicy(policy OutboundPolicy) OutboundPolicy {
	if policy.TextChunkLimit <= 0 {
		policy.TextChunkLimit = 2000
	}
	if policy.MediaOrder == "" {
		policy.MediaOrder = OutboundOrderMediaFirst
	}
	if policy.ChunkerMode == "" {
		policy.ChunkerMode = ChunkerModeText
	}
	if policy.RetryMax <= 0 {
		policy.RetryMax = 3
	}
	if policy.RetryBackoffMs <= 0 {
		policy.RetryBackoffMs = 500
	}
	if policy.Chunker == nil {
		policy.Chunker = DefaultChunker(policy.ChunkerMode)
	}
	return policy
}

func DefaultChunker(mode ChunkerMode) Chunker {
	switch mode {
	case ChunkerModeMarkdown:
		return ChunkMarkdownText
	default:
		return ChunkText
	}
}

func ChunkText(text string, limit int) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if limit <= 0 || runeLen(trimmed) <= limit {
		return []string{trimmed}
	}
	lines := strings.Split(trimmed, "\n")
	chunks := make([]string, 0)
	buf := make([]string, 0, len(lines))
	bufLen := 0
	for _, line := range lines {
		lineLen := runeLen(line)
		sepLen := 0
		if len(buf) > 0 {
			sepLen = 1
		}
		if bufLen+sepLen+lineLen <= limit {
			buf = append(buf, line)
			bufLen += sepLen + lineLen
			continue
		}
		if len(buf) > 0 {
			chunks = append(chunks, strings.Join(buf, "\n"))
			buf = buf[:0]
			bufLen = 0
		}
		if lineLen <= limit {
			buf = append(buf, line)
			bufLen = lineLen
			continue
		}
		chunks = append(chunks, splitLongLine(line, limit)...)
	}
	if len(buf) > 0 {
		chunks = append(chunks, strings.Join(buf, "\n"))
	}
	return chunks
}

func ChunkMarkdownText(text string, limit int) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if limit <= 0 || runeLen(trimmed) <= limit {
		return []string{trimmed}
	}
	paragraphs := strings.Split(trimmed, "\n\n")
	chunks := make([]string, 0)
	buf := make([]string, 0, len(paragraphs))
	bufLen := 0
	for _, para := range paragraphs {
		paraLen := runeLen(para)
		sepLen := 0
		if len(buf) > 0 {
			sepLen = 2
		}
		if bufLen+sepLen+paraLen <= limit {
			buf = append(buf, para)
			bufLen += sepLen + paraLen
			continue
		}
		if len(buf) > 0 {
			chunks = append(chunks, strings.Join(buf, "\n\n"))
			buf = buf[:0]
			bufLen = 0
		}
		if paraLen <= limit {
			buf = append(buf, para)
			bufLen = paraLen
			continue
		}
		chunks = append(chunks, ChunkText(para, limit)...)
	}
	if len(buf) > 0 {
		chunks = append(chunks, strings.Join(buf, "\n\n"))
	}
	return chunks
}

func runeLen(value string) int {
	return len([]rune(value))
}

func splitLongLine(line string, limit int) []string {
	if limit <= 0 {
		return []string{line}
	}
	runes := []rune(line)
	chunks := make([]string, 0)
	for start := 0; start < len(runes); start += limit {
		end := start + limit
		if end > len(runes) {
			end = len(runes)
		}
		segment := strings.TrimSpace(string(runes[start:end]))
		if segment == "" {
			continue
		}
		chunks = append(chunks, segment)
	}
	return chunks
}
