package timezone

import "testing"

func TestResolve_DefaultsToUTC(t *testing.T) {
	t.Parallel()

	loc, name, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if name != DefaultName {
		t.Fatalf("name = %q, want %q", name, DefaultName)
	}
	if loc != nil && loc.String() != DefaultName {
		t.Fatalf("location = %q, want %q", loc.String(), DefaultName)
	}
}

func TestResolve_LoadsLocation(t *testing.T) {
	t.Parallel()

	loc, name, err := Resolve("Asia/Tokyo")
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if name != "Asia/Tokyo" {
		t.Fatalf("name = %q, want Asia/Tokyo", name)
	}
	if loc == nil {
		t.Fatal("location is nil")
	}
}

func TestResolve_RejectsInvalidLocation(t *testing.T) {
	t.Parallel()

	if _, _, err := Resolve("not-a-real-timezone"); err == nil {
		t.Fatal("expected error for invalid timezone")
	}
}
