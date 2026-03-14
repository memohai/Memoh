package matrix

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/memohai/memoh/internal/channel"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

const matrixHTMLFormat = "org.matrix.custom.html"

var matrixMarkdownRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(
		html.WithHardWraps(),
	),
)

type matrixFormattedMessage struct {
	Body          string
	FormattedBody string
	HasHTML       bool
}

var (
	matrixTaskListPattern = regexp.MustCompile(`^(\s*(?:[-*+]\s+|\d+\.\s+))\[( |x|X)\]\s+(.*)$`)
	matrixTableAlignCell  = regexp.MustCompile(`^:?-{3,}:?$`)
)

func formatMatrixMessage(msg channel.Message) matrixFormattedMessage {
	body := strings.TrimSpace(msg.PlainText())
	formatted := matrixFormattedMessage{Body: body}
	if msg.Format != channel.MessageFormatMarkdown || body == "" {
		return formatted
	}
	body = normalizeMatrixMarkdown(body)
	formatted.Body = body
	htmlBody, err := renderMatrixMarkdown(body)
	if err != nil || strings.TrimSpace(htmlBody) == "" {
		return formatted
	}
	formatted.FormattedBody = htmlBody
	formatted.HasHTML = true
	return formatted
}

func renderMatrixMarkdown(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}
	var buf bytes.Buffer
	if err := matrixMarkdownRenderer.Convert([]byte(text), &buf); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

func normalizeMatrixMarkdown(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	result := make([]string, 0, len(lines))
	inFence := false
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if isFenceLine(trimmed) {
			inFence = !inFence
			result = append(result, line)
			continue
		}
		if !inFence && i+1 < len(lines) && isMarkdownTableHeader(line, lines[i+1]) {
			block := []string{line, lines[i+1]}
			i += 2
			for i < len(lines) && isMarkdownTableRow(lines[i]) {
				block = append(block, lines[i])
				i++
			}
			i--
			result = append(result, "```text")
			result = append(result, block...)
			result = append(result, "```")
			continue
		}
		if !inFence {
			line = normalizeMatrixTaskListLine(line)
		}
		result = append(result, line)
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

func normalizeMatrixTaskListLine(line string) string {
	matches := matrixTaskListPattern.FindStringSubmatch(line)
	if len(matches) != 4 {
		return line
	}
	box := "☐"
	if strings.EqualFold(matches[2], "x") {
		box = "☑"
	}
	return matches[1] + box + " " + matches[3]
}

func isFenceLine(line string) bool {
	return strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~")
}

func isMarkdownTableHeader(headerLine, delimiterLine string) bool {
	if !strings.Contains(headerLine, "|") {
		return false
	}
	return isMarkdownTableDelimiter(delimiterLine)
}

func isMarkdownTableDelimiter(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.Contains(trimmed, "|") {
		return false
	}
	parts := strings.Split(trimmed, "|")
	validCells := 0
	for _, part := range parts {
		cell := strings.TrimSpace(part)
		if cell == "" {
			continue
		}
		if !matrixTableAlignCell.MatchString(cell) {
			return false
		}
		validCells++
	}
	return validCells >= 1
}

func isMarkdownTableRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed != "" && strings.Contains(trimmed, "|")
}
