package handlers

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type testFlusher struct{}

func (f *testFlusher) Flush() {}

func TestParseSinceParam(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	parsed, ok, err := parseSinceParam(now.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("parse RFC3339 failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected parseSinceParam ok=true")
	}
	if !parsed.Equal(now) {
		t.Fatalf("expected parsed time %s, got %s", now, parsed)
	}

	parsedEpoch, ok, err := parseSinceParam("1735689600000")
	if err != nil {
		t.Fatalf("parse epoch millis failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected epoch parse ok=true")
	}
	if parsedEpoch.UnixMilli() != 1735689600000 {
		t.Fatalf("expected parsed epoch millis 1735689600000, got %d", parsedEpoch.UnixMilli())
	}

	if _, _, err := parseSinceParam("invalid-time"); err == nil {
		t.Fatalf("expected invalid since parameter error")
	}
}

func TestParseBeforeParam(t *testing.T) {
	t.Parallel()

	if _, ok := parseBeforeParam(""); ok {
		t.Fatalf("expected empty before value to be ignored")
	}
	parsed, ok := parseBeforeParam("1735689600000")
	if !ok {
		t.Fatalf("expected epoch millis before value to parse")
	}
	if parsed.UnixMilli() != 1735689600000 {
		t.Fatalf("expected parsed epoch millis 1735689600000, got %d", parsed.UnixMilli())
	}
}

func TestWriteSSEJSON(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	writer := bufio.NewWriter(&output)
	flusher := &testFlusher{}

	if err := writeSSEJSON(writer, flusher, map[string]any{"type": "ping"}); err != nil {
		t.Fatalf("writeSSEJSON failed: %v", err)
	}
	raw := output.String()
	if !strings.HasPrefix(raw, "data: ") {
		t.Fatalf("expected SSE data prefix, got %q", raw)
	}
	if !strings.HasSuffix(raw, "\n\n") {
		t.Fatalf("expected SSE payload suffix, got %q", raw)
	}
	payloadText := strings.TrimSuffix(strings.TrimPrefix(raw, "data: "), "\n\n")
	var payload map[string]any
	if err := json.Unmarshal([]byte(payloadText), &payload); err != nil {
		t.Fatalf("decode SSE payload failed: %v", err)
	}
	if payload["type"] != "ping" {
		t.Fatalf("expected payload type ping, got %#v", payload["type"])
	}
}
