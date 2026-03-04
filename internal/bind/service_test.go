package bind

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

func TestParseUUID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"empty", "", true},
		{"blank", "   ", true},
		{"invalid", "not-a-uuid", true},
		{"valid", "550e8400-e29b-41d4-a716-446655440000", false},
		{"valid with spaces", "  550e8400-e29b-41d4-a716-446655440000  ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.ParseUUID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseUUID() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !got.Valid {
				t.Error("expected valid UUID")
			}
		})
	}
}

func TestIsUniqueViolation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"other error", assertAnError(), false},
		{"unique violation token", &pgconn.PgError{Code: "23505", ConstraintName: "channel_identity_bind_codes_token_unique"}, true},
		{"unique violation empty constraint", &pgconn.PgError{Code: "23505", ConstraintName: ""}, true},
		{"wrong code", &pgconn.PgError{Code: "23503", ConstraintName: "some_fk"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUniqueViolation(tt.err); got != tt.want {
				t.Errorf("isUniqueViolation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizePlatform(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"", ""},
		{"  Feishu  ", "feishu"},
		{"TELEGRAM", "telegram"},
	}
	for _, tt := range tests {
		if got := normalizePlatform(tt.raw); got != tt.want {
			t.Errorf("normalizePlatform(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestToCode(t *testing.T) {
	pgID, err := db.ParseUUID("550e8400-e29b-41d4-a716-446655440000")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	var usedBy pgtype.UUID
	_ = usedBy.Scan("660e8400-e29b-41d4-a716-446655440001")
	row := sqlc.ChannelIdentityBindCode{
		ID:                      pgID,
		Token:                   "ABC12345",
		IssuedByUserID:          pgID,
		ChannelType:             pgtype.Text{String: " Feishu ", Valid: true},
		ExpiresAt:               pgtype.Timestamptz{Time: now, Valid: true},
		UsedAt:                  pgtype.Timestamptz{Time: now, Valid: true},
		UsedByChannelIdentityID: usedBy,
		CreatedAt:               pgtype.Timestamptz{Time: now, Valid: true},
	}

	c := toCode(row)
	if c.Token != "ABC12345" {
		t.Errorf("Token = %q", c.Token)
	}
	if c.Platform != "feishu" {
		t.Errorf("Platform = %q (normalized)", c.Platform)
	}
	if c.IssuedByUserID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("IssuedByUserID = %q", c.IssuedByUserID)
	}
	if c.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be set")
	}
	if c.UsedAt.IsZero() {
		t.Error("UsedAt should be set")
	}
	if c.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
}

func TestToCode_OptionalFields(t *testing.T) {
	pgID, err := db.ParseUUID("550e8400-e29b-41d4-a716-446655440000")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	row := sqlc.ChannelIdentityBindCode{
		ID:             pgID,
		Token:          "TOKEN",
		IssuedByUserID: pgID,
		ChannelType:    pgtype.Text{Valid: false},
		ExpiresAt:      pgtype.Timestamptz{Valid: false},
		UsedAt:         pgtype.Timestamptz{Valid: false},
		CreatedAt:      pgtype.Timestamptz{Time: now, Valid: true},
	}
	c := toCode(row)
	if c.Platform != "" {
		t.Errorf("Platform should be empty, got %q", c.Platform)
	}
	if !c.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be zero")
	}
	if !c.UsedAt.IsZero() {
		t.Error("UsedAt should be zero")
	}
}

func assertAnError() error {
	return errForTest
}

var errForTest = errTyp{msg: "test error"}

type errTyp struct{ msg string }

func (e errTyp) Error() string { return e.msg }

func TestService_Issue_NilQueries(t *testing.T) {
	svc := NewService(nil, nil, nil)
	_, err := svc.Issue(context.Background(), "550e8400-e29b-41d4-a716-446655440000", "feishu", time.Hour)
	if err == nil {
		t.Fatal("expected error when queries nil")
	}
}

func TestService_Issue_InvalidUserID(t *testing.T) {
	svc := NewService(nil, nil, nil)
	_, err := svc.Issue(context.Background(), "invalid", "feishu", time.Hour)
	if err == nil {
		t.Fatal("expected error for invalid user id")
	}
}

func TestService_Get_NilQueries(t *testing.T) {
	svc := NewService(nil, nil, nil)
	_, err := svc.Get(context.Background(), "TOKEN")
	if err == nil {
		t.Fatal("expected error when queries nil")
	}
}

func TestService_Consume_NilConfig(t *testing.T) {
	svc := NewService(nil, nil, nil)
	code := Code{Token: "ABC", IssuedByUserID: "550e8400-e29b-41d4-a716-446655440000"}
	err := svc.Consume(context.Background(), code, "660e8400-e29b-41d4-a716-446655440001")
	if err == nil {
		t.Fatal("expected error when service not configured")
	}
}

// Consume fast-path (CodeUsed, CodeExpired, EmptyToken) runs after nil check; covered by integration tests.

func TestService_Consume_EmptyChannelIdentityID(t *testing.T) {
	svc := NewService(nil, nil, nil)
	code := Code{Token: "ABC", IssuedByUserID: "550e8400-e29b-41d4-a716-446655440000"}
	err := svc.Consume(context.Background(), code, "")
	if err == nil {
		t.Fatal("expected error when channel identity id empty")
	}
}

func TestService_Consume_InvalidChannelIdentityID(t *testing.T) {
	svc := NewService(nil, nil, nil)
	code := Code{Token: "ABC", IssuedByUserID: "550e8400-e29b-41d4-a716-446655440000"}
	err := svc.Consume(context.Background(), code, "not-a-uuid")
	if err == nil {
		t.Fatal("expected error for invalid channel identity id")
	}
}
