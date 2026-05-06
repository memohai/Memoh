package accounts

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"golang.org/x/crypto/bcrypt"

	tzutil "github.com/memohai/memoh/internal/timezone"
)

type Store interface {
	Get(ctx context.Context, userID string) (Account, error)
	GetPasswordIdentityBySubject(ctx context.Context, subject string) (PasswordIdentity, error)
	GetPasswordIdentityByEmail(ctx context.Context, email string) (PasswordIdentity, error)
	GetPasswordIdentityByUserID(ctx context.Context, userID string) (PasswordIdentity, error)
	ListAccounts(ctx context.Context) ([]Account, error)
	SearchAccounts(ctx context.Context, query string, limit int) ([]Account, error)
	UpsertAccount(ctx context.Context, input UpsertAccountInput) (Account, error)
	CreateHuman(ctx context.Context, input CreateHumanInput) (Account, error)
	UpsertPasswordIdentity(ctx context.Context, input UpsertPasswordIdentityInput) (PasswordIdentity, error)
	UpdateAdmin(ctx context.Context, input UpdateAdminInput) (Account, error)
	UpdateProfile(ctx context.Context, input UpdateProfileInput) (Account, error)
	UpdatePasswordIdentity(ctx context.Context, identityID string, credentialSecret string) error
	TouchLogin(ctx context.Context, userID string, identityID string) error
}

type Service struct {
	store  Store
	logger *slog.Logger
}

var (
	ErrNotFound           = errors.New("account not found")
	ErrInvalidPassword    = errors.New("invalid password")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInactiveAccount    = errors.New("account is inactive")
)

func NewService(log *slog.Logger, store Store) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		store:  store,
		logger: log.With(slog.String("service", "iam.accounts")),
	}
}

func (s *Service) Get(ctx context.Context, userID string) (Account, error) {
	if s.store == nil {
		return Account{}, errors.New("account store not configured")
	}
	return s.store.Get(ctx, strings.TrimSpace(userID))
}

func (s *Service) Login(ctx context.Context, identity, password string) (Account, error) {
	if s.store == nil {
		return Account{}, errors.New("account store not configured")
	}
	identity = strings.TrimSpace(identity)
	if identity == "" || strings.TrimSpace(password) == "" {
		return Account{}, ErrInvalidCredentials
	}

	passwordIdentity, err := s.lookupPasswordIdentity(ctx, identity)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Account{}, ErrInvalidCredentials
		}
		return Account{}, err
	}
	if strings.TrimSpace(passwordIdentity.CredentialSecret) == "" {
		return Account{}, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordIdentity.CredentialSecret), []byte(password)); err != nil {
		return Account{}, ErrInvalidCredentials
	}

	account, err := s.store.Get(ctx, passwordIdentity.UserID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Account{}, ErrInvalidCredentials
		}
		return Account{}, err
	}
	if !account.IsActive {
		return Account{}, ErrInactiveAccount
	}
	if err := s.store.TouchLogin(ctx, account.ID, passwordIdentity.ID); err != nil && s.logger != nil {
		s.logger.Warn("touch login failed", slog.Any("error", err))
	}
	return normalizeAccount(account), nil
}

func (s *Service) ListAccounts(ctx context.Context) ([]Account, error) {
	if s.store == nil {
		return nil, errors.New("account store not configured")
	}
	accounts, err := s.store.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	for i := range accounts {
		accounts[i] = normalizeAccount(accounts[i])
	}
	return accounts, nil
}

func (s *Service) SearchAccounts(ctx context.Context, query string, limit int) ([]Account, error) {
	if s.store == nil {
		return nil, errors.New("account store not configured")
	}
	if limit <= 0 {
		limit = 50
	}
	accounts, err := s.store.SearchAccounts(ctx, strings.TrimSpace(query), limit)
	if err != nil {
		return nil, err
	}
	for i := range accounts {
		accounts[i] = normalizeAccount(accounts[i])
	}
	return accounts, nil
}

func (s *Service) Create(ctx context.Context, userID string, req CreateAccountRequest) (Account, error) {
	if s.store == nil {
		return Account{}, errors.New("account store not configured")
	}
	input, passwordHash, err := prepareCreate(req)
	if err != nil {
		return Account{}, err
	}
	input.UserID = strings.TrimSpace(userID)
	account, err := s.store.UpsertAccount(ctx, input)
	if err != nil {
		return Account{}, err
	}
	if _, err := s.store.UpsertPasswordIdentity(ctx, passwordIdentityInput(account, passwordHash)); err != nil {
		return Account{}, err
	}
	return normalizeAccount(account), nil
}

func (s *Service) CreateHuman(ctx context.Context, userID string, req CreateAccountRequest) (Account, error) {
	if s.store == nil {
		return Account{}, errors.New("account store not configured")
	}
	input, passwordHash, err := prepareCreate(req)
	if err != nil {
		return Account{}, err
	}

	var account Account
	if strings.TrimSpace(userID) == "" {
		account, err = s.store.CreateHuman(ctx, CreateHumanInput{
			Username:    input.Username,
			Email:       input.Email,
			DisplayName: input.DisplayName,
			AvatarURL:   input.AvatarURL,
			IsActive:    input.IsActive,
		})
	} else {
		input.UserID = strings.TrimSpace(userID)
		account, err = s.store.UpsertAccount(ctx, input)
	}
	if err != nil {
		return Account{}, err
	}
	if _, err := s.store.UpsertPasswordIdentity(ctx, passwordIdentityInput(account, passwordHash)); err != nil {
		return Account{}, err
	}
	return normalizeAccount(account), nil
}

func (s *Service) UpdateAdmin(ctx context.Context, userID string, req UpdateAccountRequest) (Account, error) {
	if s.store == nil {
		return Account{}, errors.New("account store not configured")
	}
	existing, err := s.store.Get(ctx, strings.TrimSpace(userID))
	if err != nil {
		return Account{}, err
	}
	displayName := strings.TrimSpace(existing.DisplayName)
	if req.DisplayName != nil {
		displayName = strings.TrimSpace(*req.DisplayName)
	}
	if displayName == "" {
		displayName = strings.TrimSpace(existing.Username)
	}
	avatarURL := strings.TrimSpace(existing.AvatarURL)
	if req.AvatarURL != nil {
		avatarURL = strings.TrimSpace(*req.AvatarURL)
	}
	isActive := existing.IsActive
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	account, err := s.store.UpdateAdmin(ctx, UpdateAdminInput{
		UserID:      existing.ID,
		DisplayName: displayName,
		AvatarURL:   avatarURL,
		IsActive:    isActive,
	})
	if err != nil {
		return Account{}, err
	}
	return normalizeAccount(account), nil
}

func (s *Service) UpdateProfile(ctx context.Context, userID string, req UpdateProfileRequest) (Account, error) {
	if s.store == nil {
		return Account{}, errors.New("account store not configured")
	}
	existing, err := s.store.Get(ctx, strings.TrimSpace(userID))
	if err != nil {
		return Account{}, err
	}
	displayName := strings.TrimSpace(existing.DisplayName)
	if req.DisplayName != nil {
		displayName = strings.TrimSpace(*req.DisplayName)
	}
	if displayName == "" {
		displayName = strings.TrimSpace(existing.Username)
	}
	avatarURL := strings.TrimSpace(existing.AvatarURL)
	if req.AvatarURL != nil {
		avatarURL = strings.TrimSpace(*req.AvatarURL)
	}
	timezone := strings.TrimSpace(existing.Timezone)
	if req.Timezone != nil {
		resolved, _, err := tzutil.Resolve(*req.Timezone)
		if err != nil {
			return Account{}, err
		}
		timezone = resolved.String()
	}
	if timezone == "" {
		timezone = "UTC"
	}
	account, err := s.store.UpdateProfile(ctx, UpdateProfileInput{
		UserID:      existing.ID,
		DisplayName: displayName,
		AvatarURL:   avatarURL,
		Timezone:    timezone,
	})
	if err != nil {
		return Account{}, err
	}
	return normalizeAccount(account), nil
}

func (s *Service) UpdatePassword(ctx context.Context, userID, currentPassword, newPassword string) error {
	if s.store == nil {
		return errors.New("account store not configured")
	}
	if strings.TrimSpace(newPassword) == "" {
		return errors.New("new password is required")
	}
	identity, err := s.store.GetPasswordIdentityByUserID(ctx, strings.TrimSpace(userID))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrInvalidPassword
		}
		return err
	}
	if strings.TrimSpace(currentPassword) == "" || strings.TrimSpace(identity.CredentialSecret) == "" {
		return ErrInvalidPassword
	}
	if err := bcrypt.CompareHashAndPassword([]byte(identity.CredentialSecret), []byte(currentPassword)); err != nil {
		return ErrInvalidPassword
	}
	passwordHash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.store.UpdatePasswordIdentity(ctx, identity.ID, passwordHash)
}

func (s *Service) ResetPassword(ctx context.Context, userID, newPassword string) error {
	if s.store == nil {
		return errors.New("account store not configured")
	}
	if strings.TrimSpace(newPassword) == "" {
		return errors.New("new password is required")
	}
	identity, err := s.store.GetPasswordIdentityByUserID(ctx, strings.TrimSpace(userID))
	if err != nil {
		return err
	}
	passwordHash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}
	return s.store.UpdatePasswordIdentity(ctx, identity.ID, passwordHash)
}

func (s *Service) lookupPasswordIdentity(ctx context.Context, identity string) (PasswordIdentity, error) {
	subject := normalizeSubject(identity)
	passwordIdentity, err := s.store.GetPasswordIdentityBySubject(ctx, subject)
	if err == nil {
		return passwordIdentity, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return PasswordIdentity{}, err
	}
	return s.store.GetPasswordIdentityByEmail(ctx, normalizeEmail(identity))
}

func prepareCreate(req CreateAccountRequest) (UpsertAccountInput, string, error) {
	username := strings.TrimSpace(req.Username)
	if username == "" {
		return UpsertAccountInput{}, "", errors.New("username is required")
	}
	if strings.TrimSpace(req.Password) == "" {
		return UpsertAccountInput{}, "", errors.New("password is required")
	}
	passwordHash, err := hashPassword(req.Password)
	if err != nil {
		return UpsertAccountInput{}, "", err
	}
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		displayName = username
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	return UpsertAccountInput{
		Username:    username,
		Email:       normalizeEmail(req.Email),
		DisplayName: displayName,
		AvatarURL:   strings.TrimSpace(req.AvatarURL),
		IsActive:    isActive,
	}, passwordHash, nil
}

func passwordIdentityInput(account Account, passwordHash string) UpsertPasswordIdentityInput {
	return UpsertPasswordIdentityInput{
		UserID:           account.ID,
		Subject:          normalizeSubject(account.Username),
		Email:            normalizeEmail(account.Email),
		Username:         strings.TrimSpace(account.Username),
		DisplayName:      strings.TrimSpace(account.DisplayName),
		AvatarURL:        strings.TrimSpace(account.AvatarURL),
		CredentialSecret: passwordHash,
	}
}

func hashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func normalizeAccount(account Account) Account {
	account.Username = strings.TrimSpace(account.Username)
	account.Email = normalizeEmail(account.Email)
	account.DisplayName = strings.TrimSpace(account.DisplayName)
	if account.DisplayName == "" {
		account.DisplayName = account.Username
	}
	account.AvatarURL = strings.TrimSpace(account.AvatarURL)
	account.Timezone = strings.TrimSpace(account.Timezone)
	return account
}

func normalizeSubject(subject string) string {
	return strings.ToLower(strings.TrimSpace(subject))
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
