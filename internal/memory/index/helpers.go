package index

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	return limit
}

func vectorLiteral(values []float32) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strconv.FormatFloat(float64(value), 'g', -1, 32))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func payloadJSON(payload map[string]string) ([]byte, error) {
	if payload == nil {
		payload = map[string]string{}
	}
	return json.Marshal(payload)
}

func parsePayload(raw []byte) map[string]string {
	if len(raw) == 0 {
		return map[string]string{}
	}
	var payload map[string]string
	if err := json.Unmarshal(raw, &payload); err != nil || payload == nil {
		return map[string]string{}
	}
	return payload
}

func querySparsePairs(vec SparseVector) ([]byte, error) {
	if len(vec.Indices) != len(vec.Values) {
		return nil, fmt.Errorf("sparse vector length mismatch: %d indices, %d values", len(vec.Indices), len(vec.Values))
	}
	pairs := make([][2]float64, 0, len(vec.Indices))
	for i, index := range vec.Indices {
		value := vec.Values[i]
		if value == 0 {
			continue
		}
		pairs = append(pairs, [2]float64{float64(index), float64(value)})
	}
	return json.Marshal(pairs)
}

func payloadValue(payload map[string]string, key string) string {
	if payload == nil {
		return ""
	}
	return strings.TrimSpace(payload[key])
}

func denseVectorTable(dimensions int) (string, error) {
	if dimensions <= 0 {
		return "", fmt.Errorf("dense dimensions must be positive, got %d", dimensions)
	}
	return fmt.Sprintf("memory_dense_vec_%d", dimensions), nil
}
