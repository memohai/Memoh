package users

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"github.com/memohai/memoh/internal/db/sqlc"
)

type Service struct {
	queries *sqlc.Queries
	logger  *slog.Logger
}

var (
	ErrInvalidPassword    = errors.New("invalid password")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInactiveUser       = errors.New("user is inactive")
)

func NewService(log *slog.Logger, queries *sqlc.Queries) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		queries: queries,
		logger:  log.With(slog.String("service", "users")),
	}
}

func (s *Service) Get(ctx context.Context, userID string) (User, error) {
	if s.queries == nil {
		return User{}, fmt.Errorf("user queries not configured")
	}
	pgID, err := parseUUID(userID)
	if err != nil {
		return User{}, err
	}
	row, err := s.queries.GetUserByID(ctx, pgID)
	if err != nil {
		return User{}, err
	}
	return toUser(row), nil
}

func (s *Service) Login(ctx context.Context, identity, password string) (User, error) {
	if s.queries == nil {
		return User{}, fmt.Errorf("user queries not configured")
	}
	identity = strings.TrimSpace(identity)
	if identity == "" || strings.TrimSpace(password) == "" {
		return User{}, ErrInvalidCredentials
	}
	row, err := s.queries.GetUserByIdentity(ctx, identity)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrInvalidCredentials
		}
		return User{}, err
	}
	if !row.IsActive {
		return User{}, ErrInactiveUser
	}
	if err := bcrypt.CompareHashAndPassword([]byte(row.PasswordHash), []byte(password)); err != nil {
		return User{}, ErrInvalidCredentials
	}
	if _, err := s.queries.UpdateUserLastLogin(ctx, row.ID); err != nil {
		if s.logger != nil {
			s.logger.Warn("touch last login failed", slog.Any("error", err))
		}
	}
	return toUser(row), nil
}

func (s *Service) ListUsers(ctx context.Context) ([]User, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("user queries not configured")
	}
	rows, err := s.queries.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]User, 0, len(rows))
	for _, row := range rows {
		items = append(items, toUser(row))
	}
	return items, nil
}

func (s *Service) ListUsersByType(ctx context.Context, userType string) ([]User, error) {
	if s.queries == nil {
		return nil, fmt.Errorf("user queries not configured")
	}
	return nil, fmt.Errorf("user type filtering is not supported")
}

func (s *Service) IsAdmin(ctx context.Context, userID string) (bool, error) {
	if s.queries == nil {
		return false, fmt.Errorf("user queries not configured")
	}
	pgID, err := parseUUID(userID)
	if err != nil {
		return false, err
	}
	row, err := s.queries.GetUserByID(ctx, pgID)
	if err != nil {
		return false, err
	}
	return isAdminRole(row.Role), nil
}

func (s *Service) CreateHuman(ctx context.Context, req CreateUserRequest) (User, error) {
	if s.queries == nil {
		return User{}, fmt.Errorf("user queries not configured")
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		return User{}, fmt.Errorf("username is required")
	}
	password := strings.TrimSpace(req.Password)
	if password == "" {
		return User{}, fmt.Errorf("password is required")
	}
	role, err := normalizeRole(req.Role)
	if err != nil {
		return User{}, err
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
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

	emailValue := pgtype.Text{Valid: false}
	if email != "" {
		emailValue = pgtype.Text{String: email, Valid: true}
	}
	displayValue := pgtype.Text{String: displayName, Valid: displayName != ""}
	avatarValue := pgtype.Text{Valid: false}
	if avatarURL != "" {
		avatarValue = pgtype.Text{String: avatarURL, Valid: true}
	}

	row, err := s.queries.CreateUser(ctx, sqlc.CreateUserParams{
		Username:     username,
		Email:        emailValue,
		PasswordHash: string(hashed),
		Role:         role,
		DisplayName:  displayValue,
		AvatarUrl:    avatarValue,
		IsActive:     isActive,
		DataRoot:     pgtype.Text{Valid: false},
	})
	if err != nil {
		return User{}, err
	}
	return toUser(row), nil
}

func (s *Service) UpdateUserAdmin(ctx context.Context, userID string, req UpdateUserRequest) (User, error) {
	if s.queries == nil {
		return User{}, fmt.Errorf("user queries not configured")
	}
	pgID, err := parseUUID(userID)
	if err != nil {
		return User{}, err
	}
	existing, err := s.queries.GetUserByID(ctx, pgID)
	if err != nil {
		return User{}, err
	}
	role := fmt.Sprint(existing.Role)
	if req.Role != nil {
		role, err = normalizeRole(*req.Role)
		if err != nil {
			return User{}, err
		}
	}
	displayName := strings.TrimSpace(existing.DisplayName.String)
	if req.DisplayName != nil {
		displayName = strings.TrimSpace(*req.DisplayName)
	}
	if displayName == "" {
		displayName = existing.Username
	}
	avatarURL := strings.TrimSpace(existing.AvatarUrl.String)
	if req.AvatarURL != nil {
		avatarURL = strings.TrimSpace(*req.AvatarURL)
	}
	isActive := existing.IsActive
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	row, err := s.queries.UpdateUserAdmin(ctx, sqlc.UpdateUserAdminParams{
		ID:          pgID,
		Role:        role,
		DisplayName: pgtype.Text{String: displayName, Valid: displayName != ""},
		AvatarUrl:   pgtype.Text{String: avatarURL, Valid: avatarURL != ""},
		IsActive:    isActive,
	})
	if err != nil {
		return User{}, err
	}
	return toUser(row), nil
}

func (s *Service) UpdateProfile(ctx context.Context, userID string, req UpdateProfileRequest) (User, error) {
	if s.queries == nil {
		return User{}, fmt.Errorf("user queries not configured")
	}
	pgID, err := parseUUID(userID)
	if err != nil {
		return User{}, err
	}
	existing, err := s.queries.GetUserByID(ctx, pgID)
	if err != nil {
		return User{}, err
	}
	displayName := strings.TrimSpace(existing.DisplayName.String)
	if req.DisplayName != nil {
		displayName = strings.TrimSpace(*req.DisplayName)
	}
	if displayName == "" {
		displayName = existing.Username
	}
	avatarURL := strings.TrimSpace(existing.AvatarUrl.String)
	if req.AvatarURL != nil {
		avatarURL = strings.TrimSpace(*req.AvatarURL)
	}
	row, err := s.queries.UpdateUserProfile(ctx, sqlc.UpdateUserProfileParams{
		ID:          pgID,
		DisplayName: pgtype.Text{String: displayName, Valid: displayName != ""},
		AvatarUrl:   pgtype.Text{String: avatarURL, Valid: avatarURL != ""},
		IsActive:    existing.IsActive,
	})
	if err != nil {
		return User{}, err
	}
	return toUser(row), nil
}

func (s *Service) UpdatePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	if s.queries == nil {
		return fmt.Errorf("user queries not configured")
	}
	if strings.TrimSpace(newPassword) == "" {
		return fmt.Errorf("new password is required")
	}
	pgID, err := parseUUID(userID)
	if err != nil {
		return err
	}
	existing, err := s.queries.GetUserByID(ctx, pgID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(currentPassword) == "" {
		return ErrInvalidPassword
	}
	if err := bcrypt.CompareHashAndPassword([]byte(existing.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidPassword
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.queries.UpdateUserPassword(ctx, sqlc.UpdateUserPasswordParams{
		ID:           pgID,
		PasswordHash: string(hashed),
	})
	return err
}

func (s *Service) ResetPassword(ctx context.Context, userID, newPassword string) error {
	if s.queries == nil {
		return fmt.Errorf("user queries not configured")
	}
	if strings.TrimSpace(newPassword) == "" {
		return fmt.Errorf("new password is required")
	}
	pgID, err := parseUUID(userID)
	if err != nil {
		return err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.queries.UpdateUserPassword(ctx, sqlc.UpdateUserPasswordParams{
		ID:           pgID,
		PasswordHash: string(hashed),
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

func toUser(row sqlc.User) User {
	email := ""
	if row.Email.Valid {
		email = row.Email.String
	}
	displayName := ""
	if row.DisplayName.Valid {
		displayName = row.DisplayName.String
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
	return User{
		ID:          toUUIDString(row.ID),
		Username:    row.Username,
		Email:       email,
		Role:        fmt.Sprint(row.Role),
		DisplayName: displayName,
		AvatarURL:   avatarURL,
		IsActive:    row.IsActive,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		LastLoginAt: lastLogin,
	}
}

func parseUUID(id string) (pgtype.UUID, error) {
	parsed, err := uuid.Parse(strings.TrimSpace(id))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid UUID: %w", err)
	}
	var pgID pgtype.UUID
	pgID.Valid = true
	copy(pgID.Bytes[:], parsed[:])
	return pgID, nil
}

func toUUIDString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	parsed, err := uuid.FromBytes(value.Bytes[:])
	if err != nil {
		return ""
	}
	return parsed.String()
}
