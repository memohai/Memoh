package db

import (
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/config"
)

// TestAppDSNUsesRuntimeRole asserts AppDSN connects as the restricted runtime
// role (memoh_app) while DSN keeps the owner/migration role. The two DSNs must
// differ in their userinfo so the runtime pool is a non-owner connection.
func TestAppDSNUsesRuntimeRole(t *testing.T) {
	cfg := config.PostgresConfig{
		Host:        "h",
		Port:        5432,
		User:        "memoh",
		Password:    "owner",
		AppUser:     "memoh_app",
		AppPassword: "apppw",
		Database:    "memoh",
		SSLMode:     "disable",
	}

	appDSN := AppDSN(cfg)
	if !strings.Contains(appDSN, "memoh_app") {
		t.Fatalf("AppDSN = %q, want runtime role memoh_app", appDSN)
	}
	if !strings.Contains(appDSN, "apppw") {
		t.Fatalf("AppDSN = %q, want runtime password", appDSN)
	}

	ownerDSN := DSN(cfg)
	if strings.Contains(ownerDSN, "memoh_app") {
		t.Fatalf("DSN = %q, must not use runtime role", ownerDSN)
	}
	if !strings.Contains(ownerDSN, "memoh") {
		t.Fatalf("DSN = %q, want owner role", ownerDSN)
	}
	if appDSN == ownerDSN {
		t.Fatalf("AppDSN and DSN are identical (%q); AppDSN must use a different role", appDSN)
	}
}

// TestAppDSNFallsBackToOwner covers OSS first-boot before the memoh_app role
// exists: when AppUser/AppPassword are unset, AppDSN falls back to the owner DSN
// so the server can start and run migrations that create the role.
func TestAppDSNFallsBackToOwner(t *testing.T) {
	cfg := config.PostgresConfig{
		Host:     "h",
		Port:     5432,
		User:     "memoh",
		Password: "owner",
		Database: "memoh",
		SSLMode:  "disable",
	}
	if got, want := AppDSN(cfg), DSN(cfg); got != want {
		t.Fatalf("AppDSN fallback = %q, want owner DSN %q", got, want)
	}
}
