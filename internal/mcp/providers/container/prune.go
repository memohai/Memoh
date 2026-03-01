package container

import (
	"strings"
	"unicode/utf8"

	textprune "github.com/memohai/memoh/internal/prune"
)

// Output pruning limits for tool results.
const (
	toolOutputHeadBytes = 4 * 1024
	toolOutputTailBytes = 1 * 1024
	toolOutputHeadLines = 150
	toolOutputTailLines = 50
)

// Read tool limits - single conservative budget.
// AI can paginate via line_offset/n_lines if file is larger.
const (
	readMaxLines      = 200  // Max lines per read
	readMaxBytes      = 5120 // 5KB per read
	readMaxLineLength = 1000 // Max characters per line (runes)
	readHeadBytes     = 3072 // 3KB head when pruning
	readTailBytes     = 1024 // 1KB tail when pruning
	readHeadLines     = 120  // 120 lines head when pruning
	readTailLines     = 40   // 40 lines tail when pruning
)

// pruneToolOutputText prunes generic tool output (exec, etc.).
func pruneToolOutputText(text, label string) string {
	return textprune.PruneWithEdges(text, label, textprune.Config{
		MaxBytes:  textprune.DefaultMaxBytes,
		MaxLines:  textprune.DefaultMaxLines,
		HeadBytes: toolOutputHeadBytes,
		TailBytes: toolOutputTailBytes,
		HeadLines: toolOutputHeadLines,
		TailLines: toolOutputTailLines,
		Marker:    textprune.DefaultMarker,
	})
}

// pruneReadOutput prunes read tool output.
func pruneReadOutput(text string) string {
	return textprune.PruneWithEdges(text, "read output", textprune.Config{
		MaxBytes:  readMaxBytes,
		MaxLines:  readMaxLines,
		HeadBytes: readHeadBytes,
		TailBytes: readTailBytes,
		HeadLines: readHeadLines,
		TailLines: readTailLines,
		Marker:    textprune.DefaultMarker,
	})
}

// truncateLine truncates a line to maxLength runes (not bytes) and adds ellipsis if truncated.
func truncateLine(line string, maxLength int) string {
	if maxLength <= 0 {
		return line
	}

	// Count runes, not bytes.
	runeCount := utf8.RuneCountInString(line)
	if runeCount <= maxLength {
		return line
	}

	// Find the byte position where we should cut (at maxLength runes).
	bytePos := 0
	runes := 0
	for bytePos < len(line) && runes < maxLength {
		_, size := utf8.DecodeRuneInString(line[bytePos:])
		bytePos += size
		runes++
	}

	return line[:bytePos] + "..."
}

// formatTruncatedLines formats a list of line numbers for display, collapsing consecutive numbers.
func formatTruncatedLines(lines []int) string {
	if len(lines) == 0 {
		return ""
	}
	if len(lines) == 1 {
		return itoa(lines[0])
	}
	if len(lines) <= 3 {
		parts := make([]string, len(lines))
		for i, n := range lines {
			parts[i] = itoa(n)
		}
		return strings.Join(parts, ", ")
	}
	// For many truncated lines, show count and examples.
	return itoa(lines[0]) + ", " + itoa(lines[1]) + ", " + itoa(lines[2]) + "... (" + itoa(len(lines)) + " total)"
}

// itoa converts int to string without allocation.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	sign := n < 0
	var u uint64
	if sign {
		// Avoid overflow for MinInt.
		u = uint64(-(n + 1))
		u++
	} else {
		u = uint64(n)
	}
	for u > 0 {
		i--
		buf[i] = byte('0' + u%10)
		u /= 10
	}
	if sign {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
