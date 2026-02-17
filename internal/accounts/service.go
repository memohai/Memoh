// Package accounts provides user account and credential management.
package accounts

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

// Service provides account (credential) management for users.
type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

// Errors returned by account operations.
var (
	ErrInvalidPassword    = errors.New("invalid password")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInactiveAccount    = errors.New("account is inactive")
)

// NewService creates a new accounts service.
func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "accounts")),
	}
}

// Get returns an account by user id.
func (s *Service) Get(ctx context.Context, userID string) (Account, error) {
	if s.queries == nil {
		return Account{}, errors.New("account queries not configured")
	}
	pgID, err := db.ParseUUID(userID)
	if err != nil {
		return Account{}, err
	}
	row, err := s.queries.GetAccountByUserID(ctx, pgID)
	if err != nil {
		return Account{}, err
	}
	return toAccount(row), nil
}

// Login authenticates by identity (username or email) and password.
func (s *Service) Login(ctx context.Context, identity, password string) (Account, error) {
	if s.queries == nil {
		return Account{}, errors.New("account queries not configured")
	}
	identity = strings.TrimSpace(identity)
	if identity == "" || strings.TrimSpace(password) == "" {
		return Account{}, ErrInvalidCredentials
	}
	row, err := s.queries.GetAccountByIdentity(ctx, pgtype.Text{String: identity, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Account{}, ErrInvalidCredentials
		}
		return Account{}, err
	}
	if !row.IsActive {
		return Account{}, ErrInactiveAccount
	}
	if !row.PasswordHash.Valid {
		return Account{}, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(row.PasswordHash.String), []byte(password)); err != nil {
		return Account{}, ErrInvalidCredentials
	}
	if _, err := s.queries.UpdateAccountLastLogin(ctx, row.ID); err != nil {
		if s.logger != nil {
			s.logger.Warn("touch last login failed", slog.Any("error", err))
		}
	}
	return toAccount(row), nil
}

// ListAccounts returns all accounts.
func (s *Service) ListAccounts(ctx context.Context) ([]Account, error) {
	if s.queries == nil {
		return nil, errors.New("account queries not configured")
	}
	rows, err := s.queries.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]Account, 0, len(rows))
	for _, row := range rows {
		items = append(items, toAccount(row))
	}
	return items, nil
}

// IsAdmin checks if the user has admin role.
func (s *Service) IsAdmin(ctx context.Context, userID string) (bool, error) {
	if s.queries == nil {
		return false, errors.New("account queries not configured")
	}
	pgID, err := db.ParseUUID(userID)
	if err != nil {
		return false, err
	}
	row, err := s.queries.GetAccountByUserID(ctx, pgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return isAdminRole(row.Role), nil
}

// Create creates a new account for an existing user.
func (s *Service) Create(ctx context.Context, userID string, req CreateAccountRequest) (Account, error) {
	if s.queries == nil {
		return Account{}, errors.New("account queries not configured")
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		return Account{}, errors.New("username is required")
	}
	password := strings.TrimSpace(req.Password)
	if password == "" {
		return Account{}, errors.New("password is required")
	}
	role, err := normalizeRole(req.Role)
	if err != nil {
		return Account{}, err
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return Account{}, err
	}

	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = username
	}
	avatarURL := strings.TrimSpace(req.AvatarURL)
	email := strings.TrimSpace(req.Email)
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	pgUserID, err := db.ParseUUID(userID)
	if err != nil {
		return Account{}, err
	}
	emailValue := pgtype.Text{Valid: false}
	if email != "" {
		emailValue = pgtype.Text{String: email, Valid: true}
	}
	displayValue := pgtype.Text{String: displayName, Valid: displayName != ""}
	avatarValue := pgtype.Text{Valid: false}
	if avatarURL != "" {
		avatarValue = pgtype.Text{String: avatarURL, Valid: true}
	}

	row, err := s.queries.CreateAccount(ctx, sqlc.CreateAccountParams{
		UserID:       pgUserID,
		Username:     pgtype.Text{String: username, Valid: true},
		Email:        emailValue,
		PasswordHash: pgtype.Text{String: string(hashed), Valid: true},
		Role:         role,
		DisplayName:  displayValue,
		AvatarUrl:    avatarValue,
		IsActive:     isActive,
		DataRoot:     pgtype.Text{Valid: false},
	})
	if err != nil {
		return Account{}, err
	}
	return toAccount(row), nil
}

// CreateHuman keeps compatibility with older call sites.
//
// Deprecated: use Create directly.
func (s *Service) CreateHuman(ctx context.Context, userID string, req CreateAccountRequest) (Account, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		if s.queries == nil {
			return Account{}, errors.New("account queries not configured")
		}
		userRow, err := s.queries.CreateUser(ctx, sqlc.CreateUserParams{
			IsActive: true,
			Metadata: []byte("{}"),
		})
		if err != nil {
			return Account{}, err
		}
		if !userRow.ID.Valid {
			return Account{}, errors.New("create user: invalid id")
		}
		userID = userRow.ID.String()
	}
	return s.Create(ctx, userID, req)
}

// UpdateAdmin updates account fields as admin.
func (s *Service) UpdateAdmin(ctx context.Context, userID string, req UpdateAccountRequest) (Account, error) {
	if s.queries == nil {
		return Account{}, errors.New("account queries not configured")
	}
	pgID, err := db.ParseUUID(userID)
	if err != nil {
		return Account{}, err
	}
	existing, err := s.queries.GetAccountByUserID(ctx, pgID)
	if err != nil {
		return Account{}, err
	}
	role := existing.Role
	if req.Role != nil {
		role, err = normalizeRole(*req.Role)
		if err != nil {
			return Account{}, err
		}
	}
	displayName := strings.TrimSpace(existing.DisplayName.String)
	if req.DisplayName != nil {
		displayName = strings.TrimSpace(*req.DisplayName)
	}
	if displayName == "" {
		displayName = strings.TrimSpace(existing.Username.String)
	}
	avatarURL := strings.TrimSpace(existing.AvatarUrl.String)
	if req.AvatarURL != nil {
		avatarURL = strings.TrimSpace(*req.AvatarURL)
	}
	isActive := existing.IsActive
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	row, err := s.queries.UpdateAccountAdmin(ctx, sqlc.UpdateAccountAdminParams{
		UserID:      pgID,
		Role:        role,
		DisplayName: pgtype.Text{String: displayName, Valid: displayName != ""},
		AvatarUrl:   pgtype.Text{String: avatarURL, Valid: avatarURL != ""},
		IsActive:    isActive,
	})
	if err != nil {
		return Account{}, err
	}
	return toAccount(row), nil
}

// UpdateProfile updates the user's profile.
func (s *Service) UpdateProfile(ctx context.Context, userID string, req UpdateProfileRequest) (Account, error) {
	if s.queries == nil {
		return Account{}, errors.New("account queries not configured")
	}
	pgID, err := db.ParseUUID(userID)
	if err != nil {
		return Account{}, err
	}
	existing, err := s.queries.GetAccountByUserID(ctx, pgID)
	if err != nil {
		return Account{}, err
	}
	displayName := strings.TrimSpace(existing.DisplayName.String)
	if req.DisplayName != nil {
		displayName = strings.TrimSpace(*req.DisplayName)
	}
	if displayName == "" {
		displayName = strings.TrimSpace(existing.Username.String)
	}
	avatarURL := strings.TrimSpace(existing.AvatarUrl.String)
	if req.AvatarURL != nil {
		avatarURL = strings.TrimSpace(*req.AvatarURL)
	}
	row, err := s.queries.UpdateAccountProfile(ctx, sqlc.UpdateAccountProfileParams{
		ID:          pgID,
		DisplayName: pgtype.Text{String: displayName, Valid: displayName != ""},
		AvatarUrl:   pgtype.Text{String: avatarURL, Valid: avatarURL != ""},
		IsActive:    existing.IsActive,
	})
	if err != nil {
		return Account{}, err
	}
	return toAccount(row), nil
}

// UpdatePassword changes the password after verifying the current one.
func (s *Service) UpdatePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	if s.queries == nil {
		return errors.New("account queries not configured")
	}
	if strings.TrimSpace(newPassword) == "" {
		return errors.New("new password is required")
	}
	pgID, err := db.ParseUUID(userID)
	if err != nil {
		return err
	}
	existing, err := s.queries.GetAccountByUserID(ctx, pgID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(currentPassword) == "" {
		return ErrInvalidPassword
	}
	if !existing.PasswordHash.Valid {
		return ErrInvalidPassword
	}
	if err := bcrypt.CompareHashAndPassword([]byte(existing.PasswordHash.String), []byte(currentPassword)); err != nil {
		return ErrInvalidPassword
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.queries.UpdateAccountPassword(ctx, sqlc.UpdateAccountPasswordParams{
		ID:           pgID,
		PasswordHash: pgtype.Text{String: string(hashed), Valid: true},
	})
	return err
}

// ResetPassword sets a new password without requiring the current one.
func (s *Service) ResetPassword(ctx context.Context, userID, newPassword string) error {
	if s.queries == nil {
		return errors.New("account queries not configured")
	}
	if strings.TrimSpace(newPassword) == "" {
		return errors.New("new password is required")
	}
	pgID, err := db.ParseUUID(userID)
	if err != nil {
		return err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.queries.UpdateAccountPassword(ctx, sqlc.UpdateAccountPasswordParams{
		ID:           pgID,
		PasswordHash: pgtype.Text{String: string(hashed), Valid: true},
	})
	return err
}

func normalizeRole(raw string) (string, error) {
	role := strings.ToLower(strings.TrimSpace(raw))
	if role == "" {
		return "member", nil
	}
	if role != "member" && role != "admin" {
		return "", fmt.Errorf("invalid role: %s", raw)
	}
	return role, nil
}

func isAdminRole(role any) bool {
	if role == nil {
		return false
	}
	switch v := role.(type) {
	case string:
		return strings.EqualFold(v, "admin")
	case fmt.Stringer:
		return strings.EqualFold(v.String(), "admin")
	default:
		return strings.EqualFold(fmt.Sprint(v), "admin")
	}
}

func toAccount(row sqlc.User) Account {
	username := strings.TrimSpace(row.Username.String)
	email := ""
	if row.Email.Valid {
		email = row.Email.String
	}
	displayName := ""
	if row.DisplayName.Valid {
		displayName = row.DisplayName.String
	}
	if displayName == "" {
		displayName = username
	}
	avatarURL := ""
	if row.AvatarUrl.Valid {
		avatarURL = row.AvatarUrl.String
	}
	createdAt := time.Time{}
	if row.CreatedAt.Valid {
		createdAt = row.CreatedAt.Time
	}
	updatedAt := time.Time{}
	if row.UpdatedAt.Valid {
		updatedAt = row.UpdatedAt.Time
	}
	lastLogin := time.Time{}
	if row.LastLoginAt.Valid {
		lastLogin = row.LastLoginAt.Time
	}
	return Account{
		ID:          row.ID.String(),
		Username:    username,
		Email:       email,
		Role:        row.Role,
		DisplayName: displayName,
		AvatarURL:   avatarURL,
		IsActive:    row.IsActive,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		LastLoginAt: lastLogin,
	}
}
