package store

import (
	"errors"
	"fmt"
	"testing"
)

func TestPersistenceRetrySafetyMarker(t *testing.T) {
	wantErr := errors.New("persist")
	marked := MarkPersistenceRetrySafe(wantErr)
	if !IsPersistenceRetrySafe(fmt.Errorf("wrapped: %w", marked)) {
		t.Fatal("marked persistence error was not retry safe through wrapping")
	}
	if !errors.Is(marked, wantErr) {
		t.Fatal("marked persistence error did not preserve its cause")
	}
	if IsPersistenceRetrySafe(wantErr) {
		t.Fatal("unmarked persistence error was classified as retry safe")
	}
}
