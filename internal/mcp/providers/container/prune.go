package container

import (
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
