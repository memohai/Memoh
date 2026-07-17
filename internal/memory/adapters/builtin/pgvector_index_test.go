package builtin

import "testing"

func TestCheckedPGVectorInt32(t *testing.T) {
	t.Parallel()
	if got, err := checkedPgvectorInt32("limit", 42); err != nil || got != 42 {
		t.Fatalf("checkedPgvectorInt32(42) = %d, %v", got, err)
	}
	if _, err := checkedPgvectorInt32("limit", -1); err == nil {
		t.Fatal("checkedPgvectorInt32(-1) succeeded")
	}
	if _, err := checkedPgvectorInt32("limit", int(maxPgvectorInt32)+1); err == nil {
		t.Fatal("checkedPgvectorInt32(max+1) succeeded")
	}
}
