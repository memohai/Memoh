package tools

import (
	textprune "github.com/memohai/memoh/internal/prune"
)

const (
	toolOutputHeadBytes = 4 * 1024
	toolOutputTailBytes = 1 * 1024
	toolOutputHeadLines = 150
	toolOutputTailLines = 50

	readMaxLines      = 200
	readMaxBytes      = 5120
	readMaxLineLength = 1000
	readHeadBytes     = 3072
	readTailBytes     = 1024
	readHeadLines     = 120
	readTailLines     = 40

	listMaxEntries        = 200
	listCollapseThreshold = 50
)

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
