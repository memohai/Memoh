package handlers

import (
	"testing"

	"github.com/memohai/memoh/internal/models"
)

func descriptionPointer(value string) *string { return &value }

func TestMergeDiscoveredConfigFillsOnlyMissingDescription(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		existing   *string
		discovered *string
		want       *string
		changed    bool
	}{
		{name: "fills missing", discovered: descriptionPointer("Template"), want: descriptionPointer("Template"), changed: true},
		{name: "preserves user value", existing: descriptionPointer("Custom"), discovered: descriptionPointer("Template"), want: descriptionPointer("Custom")},
		{name: "preserves explicit clear", existing: descriptionPointer(""), discovered: descriptionPointer("Template"), want: descriptionPointer("")},
		{name: "ignores missing discovery"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := mergeDiscoveredConfig(
				models.ModelConfig{Description: tt.existing},
				models.ModelConfig{Description: tt.discovered},
			)
			if changed != tt.changed {
				t.Fatalf("changed = %v, want %v", changed, tt.changed)
			}
			if tt.want == nil {
				if got.Description != nil {
					t.Fatalf("description = %q, want nil", *got.Description)
				}
				return
			}
			if got.Description == nil || *got.Description != *tt.want {
				t.Fatalf("description = %v, want %q", got.Description, *tt.want)
			}
		})
	}
}
