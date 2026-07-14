package contextbudget

import "testing"

func TestEstimateTokensRoundsUpNonEmptySources(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		size int
		want int
	}{
		{name: "empty", size: 0, want: 0},
		{name: "one byte", size: 1, want: 1},
		{name: "exact block", size: 4, want: 1},
		{name: "partial block", size: 5, want: 2},
		{name: "negative", size: -1, want: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := EstimateTokensForBytes(test.size); got != test.want {
				t.Fatalf("EstimateTokensForBytes(%d) = %d, want %d", test.size, got, test.want)
			}
		})
	}
}

func TestEstimateTextTokensUsesUTF8Bytes(t *testing.T) {
	t.Parallel()

	if got := EstimateTextTokens("你好"); got != 2 {
		t.Fatalf("EstimateTextTokens() = %d, want 2 for six UTF-8 bytes", got)
	}
}
