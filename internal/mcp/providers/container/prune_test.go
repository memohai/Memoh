package container

import (
	"strconv"
	"testing"
)

func TestItoa_MatchesStrconv(t *testing.T) {
	minInt := -int(^uint(0)>>1) - 1

	tests := []int{
		0,
		1,
		-1,
		42,
		-42,
		123456789,
		-123456789,
		minInt,
	}

	for _, n := range tests {
		got := itoa(n)
		want := strconv.Itoa(n)
		if got != want {
			t.Fatalf("itoa(%d) = %q, want %q", n, got, want)
		}
	}
}
