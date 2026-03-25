package timezone

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const DefaultName = "UTC"

// Resolve returns a loadable time.Location for the provided timezone name.
// Empty names fall back to UTC so the rest of the system stays deterministic.
func Resolve(name string) (*time.Location, string, error) {
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return time.UTC, DefaultName, nil
	}
	if strings.EqualFold(normalized, "local") {
		return time.Local, "local", nil
	}
	loc, err := time.LoadLocation(normalized)
	if err != nil {
		return nil, "", fmt.Errorf("load timezone %q: %w", normalized, err)
	}
	return loc, normalized, nil
}

// MustResolve is a convenience helper for initialization code.
func MustResolve(name string) *time.Location {
	loc, _, err := Resolve(name)
	if err != nil {
		panic(err)
	}
	if loc == nil {
		panic(errors.New("timezone location is nil"))
	}
	return loc
}
