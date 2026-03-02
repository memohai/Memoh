package flow

import "testing"

func TestParseLoopDetectionEnabledFromMetadata(t *testing.T) {
	tests := []struct {
		name     string
		payload  []byte
		expected bool
	}{
		{
			name:     "empty payload defaults to false",
			payload:  nil,
			expected: false,
		},
		{
			name:     "invalid json defaults to false",
			payload:  []byte("{"),
			expected: false,
		},
		{
			name:     "missing nested path defaults to false",
			payload:  []byte(`{"features":{}}`),
			expected: false,
		},
		{
			name:     "explicit false",
			payload:  []byte(`{"features":{"loop_detection":{"enabled":false}}}`),
			expected: false,
		},
		{
			name:     "explicit true",
			payload:  []byte(`{"features":{"loop_detection":{"enabled":true}}}`),
			expected: true,
		},
		{
			name:     "non-boolean value defaults to false",
			payload:  []byte(`{"features":{"loop_detection":{"enabled":"true"}}}`),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLoopDetectionEnabledFromMetadata(tt.payload)
			if got != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}
