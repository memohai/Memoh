package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"

	"github.com/memohai/memoh/internal/iam/accounts"
	iamauth "github.com/memohai/memoh/internal/iam/auth"
	"github.com/memohai/memoh/internal/iam/sso"
)

func TestPasswordLoginReturnsSessionTokenWithoutRole(t *testing.T) {
	handler, sessionStore, _ := newTestAuthHandler(t)
	e := echo.New()
	handler.Register(e)

	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"username":"alice","password":"secret"}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, ok := raw["role"]; ok {
		t.Fatalf("login response contains role: %v", raw)
	}
	if raw["session_id"] != "session-1" {
		t.Fatalf("session_id = %v, want session-1", raw["session_id"])
	}
	if sessionStore.created.UserID != "user-1" {
		t.Fatalf("created session user = %q, want user-1", sessionStore.created.UserID)
	}

	claims := parseTokenClaims(t, raw["access_token"].(string))
	if claims["user_id"] != "user-1" || claims["sub"] != "user-1" {
		t.Fatalf("user claims = %v", claims)
	}
	if claims["session_id"] != "session-1" {
		t.Fatalf("session_id claim = %v, want session-1", claims["session_id"])
	}
	if _, ok := claims["iat"]; !ok {
		t.Fatalf("iat claim missing: %v", claims)
	}
	if _, ok := claims["exp"]; !ok {
		t.Fatalf("exp claim missing: %v", claims)
	}
}

func TestRefreshValidatesSessionAndLogoutRevokesSession(t *testing.T) {
	handler, sessionStore, _ := newTestAuthHandler(t)
	token, _, err := iamauth.GenerateToken("user-1", "session-1", "test-secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	e := echo.New()
	e.Use(iamauth.JWTMiddleware("test-secret", func(c echo.Context) bool {
		return c.Path() == "/auth/login"
	}))
	handler.Register(e)

	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("refresh status = %d body = %s", rec.Code, rec.Body.String())
	}
	if sessionStore.validatedUserID != "user-1" || sessionStore.validatedSessionID != "session-1" {
		t.Fatalf("validated = (%q, %q)", sessionStore.validatedUserID, sessionStore.validatedSessionID)
	}

	req = httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d body = %s", rec.Code, rec.Body.String())
	}
	if sessionStore.revokedSessionID != "session-1" {
		t.Fatalf("revoked session = %q, want session-1", sessionStore.revokedSessionID)
	}
}

func TestSSOCallbackRedirectsWithLoginCodeOnlyAndExchangeReturnsToken(t *testing.T) {
	handler, _, ssoService := newTestAuthHandler(t)
	e := echo.New()
	handler.Register(e)

	startReq := httptest.NewRequest(http.MethodGet, "/auth/sso/google/start", nil)
	startRec := httptest.NewRecorder()
	e.ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("start status = %d body = %s", startRec.Code, startRec.Body.String())
	}
	stateCookie := firstCookie(startRec.Result().Cookies(), sso.OIDCStateCookieName)
	if stateCookie == nil {
		t.Fatalf("oidc state cookie missing")
	}

	callbackReq := httptest.NewRequest(http.MethodGet, "/auth/sso/google/callback?code=idp-code&state="+ssoService.startedState, nil)
	callbackReq.AddCookie(stateCookie)
	callbackRec := httptest.NewRecorder()
	e.ServeHTTP(callbackRec, callbackReq)
	if callbackRec.Code != http.StatusFound {
		t.Fatalf("callback status = %d body = %s", callbackRec.Code, callbackRec.Body.String())
	}
	location := callbackRec.Header().Get(echo.HeaderLocation)
	if !strings.Contains(location, "code=sso-code") {
		t.Fatalf("callback location = %q, want login code", location)
	}
	if strings.Contains(location, "access_token") || strings.Contains(location, "token=") {
		t.Fatalf("callback leaked token in url: %q", location)
	}

	exchangeReq := httptest.NewRequest(http.MethodPost, "/auth/sso/exchange", strings.NewReader(`{"code":"sso-code"}`))
	exchangeReq.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	exchangeRec := httptest.NewRecorder()
	e.ServeHTTP(exchangeRec, exchangeReq)
	if exchangeRec.Code != http.StatusOK {
		t.Fatalf("exchange status = %d body = %s", exchangeRec.Code, exchangeRec.Body.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(exchangeRec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode exchange: %v", err)
	}
	if _, ok := raw["role"]; ok {
		t.Fatalf("exchange response contains role: %v", raw)
	}
	if raw["access_token"] == "" || raw["session_id"] != "session-1" {
		t.Fatalf("exchange response = %v", raw)
	}
	parseTokenClaims(t, raw["access_token"].(string))
}

func newTestAuthHandler(t *testing.T) (*AuthHandler, *fakeAuthSessionStore, *fakeAuthSSOService) {
	t.Helper()
	accountStore := newFakeAccountStore(t)
	sessionStore := &fakeAuthSessionStore{}
	ssoService := &fakeAuthSSOService{}
	handler := NewAuthHandler(slog.New(slog.DiscardHandler), NewIAMAccountServiceAdapter(accounts.NewService(slog.Default(), accountStore)), "test-secret", time.Hour)
	handler.SetSessionStore(sessionStore)
	handler.SetSSOService(ssoService)
	return handler, sessionStore, ssoService
}

func parseTokenClaims(t *testing.T, tokenString string) jwt.MapClaims {
	t.Helper()
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return []byte("test-secret"), nil
	})
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if !token.Valid {
		t.Fatalf("token invalid")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatalf("claims type = %T", token.Claims)
	}
	return claims
}

func firstCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

type fakeAccountStore struct {
	account  accounts.Account
	identity accounts.PasswordIdentity
}

func newFakeAccountStore(t *testing.T) *fakeAccountStore {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	return &fakeAccountStore{
		account: accounts.Account{
			ID:          "user-1",
			Username:    "alice",
			DisplayName: "Alice",
			IsActive:    true,
		},
		identity: accounts.PasswordIdentity{
			ID:               "identity-1",
			UserID:           "user-1",
			Subject:          "alice",
			Username:         "alice",
			CredentialSecret: string(hash),
		},
	}
}

func (s *fakeAccountStore) Get(_ context.Context, userID string) (accounts.Account, error) {
	if userID == s.account.ID {
		return s.account, nil
	}
	return accounts.Account{}, accounts.ErrNotFound
}

func (s *fakeAccountStore) GetPasswordIdentityBySubject(_ context.Context, subject string) (accounts.PasswordIdentity, error) {
	if subject == s.identity.Subject {
		return s.identity, nil
	}
	return accounts.PasswordIdentity{}, accounts.ErrNotFound
}

func (*fakeAccountStore) GetPasswordIdentityByEmail(_ context.Context, _ string) (accounts.PasswordIdentity, error) {
	return accounts.PasswordIdentity{}, accounts.ErrNotFound
}

func (s *fakeAccountStore) GetPasswordIdentityByUserID(_ context.Context, userID string) (accounts.PasswordIdentity, error) {
	if userID == s.identity.UserID {
		return s.identity, nil
	}
	return accounts.PasswordIdentity{}, accounts.ErrNotFound
}

func (s *fakeAccountStore) ListAccounts(context.Context) ([]accounts.Account, error) {
	return []accounts.Account{s.account}, nil
}

func (s *fakeAccountStore) SearchAccounts(context.Context, string, int) ([]accounts.Account, error) {
	return []accounts.Account{s.account}, nil
}

func (s *fakeAccountStore) UpsertAccount(context.Context, accounts.UpsertAccountInput) (accounts.Account, error) {
	return s.account, nil
}

func (s *fakeAccountStore) CreateHuman(context.Context, accounts.CreateHumanInput) (accounts.Account, error) {
	return s.account, nil
}

func (s *fakeAccountStore) UpsertPasswordIdentity(context.Context, accounts.UpsertPasswordIdentityInput) (accounts.PasswordIdentity, error) {
	return s.identity, nil
}

func (s *fakeAccountStore) UpdateAdmin(context.Context, accounts.UpdateAdminInput) (accounts.Account, error) {
	return s.account, nil
}

func (s *fakeAccountStore) UpdateProfile(context.Context, accounts.UpdateProfileInput) (accounts.Account, error) {
	return s.account, nil
}

func (*fakeAccountStore) UpdatePasswordIdentity(context.Context, string, string) error {
	return nil
}

func (*fakeAccountStore) TouchLogin(context.Context, string, string) error {
	return nil
}

type fakeAuthSessionStore struct {
	created            AuthSessionInput
	validatedUserID    string
	validatedSessionID string
	revokedSessionID   string
}

func (s *fakeAuthSessionStore) CreateSession(_ context.Context, input AuthSessionInput) (AuthSession, error) {
	s.created = input
	return AuthSession{ID: "session-1", UserID: input.UserID, ExpiresAt: input.ExpiresAt}, nil
}

func (s *fakeAuthSessionStore) ValidateSession(_ context.Context, userID string, sessionID string) error {
	s.validatedUserID = userID
	s.validatedSessionID = sessionID
	if userID != "user-1" || sessionID != "session-1" {
		return errors.New("invalid session")
	}
	return nil
}

func (s *fakeAuthSessionStore) RevokeSession(_ context.Context, sessionID string) error {
	s.revokedSessionID = sessionID
	return nil
}

type fakeAuthSSOService struct {
	startedState string
}

func (s *fakeAuthSSOService) ListEnabledProviders(context.Context) ([]sso.Provider, error) {
	return []sso.Provider{s.provider()}, nil
}

func (s *fakeAuthSSOService) GetProvider(context.Context, string) (sso.Provider, error) {
	return s.provider(), nil
}

func (s *fakeAuthSSOService) BuildOIDCAuthRedirect(_ context.Context, _ sso.Provider, state string, _ string, _ string) (sso.OIDCAuthRedirect, error) {
	s.startedState = state
	return sso.OIDCAuthRedirect{URL: "https://idp.example.com/auth?state=" + state}, nil
}

func (*fakeAuthSSOService) CompleteOIDCCallback(_ context.Context, _ sso.Provider, _ string, _ sso.OIDCState) (sso.LoginCode, error) {
	return sso.LoginCode{Code: "sso-code", UserID: "user-1", SessionID: "session-1"}, nil
}

func (*fakeAuthSSOService) BuildSAMLAuthRedirect(context.Context, sso.Provider) (sso.SAMLAuthRedirect, error) {
	return sso.SAMLAuthRedirect{URL: "https://idp.example.com/saml", RelayState: "relay-1", RequestID: "request-1"}, nil
}

func (*fakeAuthSSOService) CompleteSAMLACS(context.Context, sso.Provider, *http.Request, sso.SAMLState) (sso.LoginCode, error) {
	return sso.LoginCode{Code: "sso-code", UserID: "user-1", SessionID: "session-1"}, nil
}

func (*fakeAuthSSOService) BuildSAMLMetadata(context.Context, sso.Provider) (string, error) {
	return "<EntityDescriptor/>", nil
}

func (*fakeAuthSSOService) ExchangeLoginCode(_ context.Context, code string) (sso.LoginCode, error) {
	if code != "sso-code" {
		return sso.LoginCode{}, errors.New("invalid code")
	}
	return sso.LoginCode{Code: code, UserID: "user-1", SessionID: "session-1"}, nil
}

func (*fakeAuthSSOService) provider() sso.Provider {
	return sso.Provider{
		ID:      "provider-1",
		Key:     "google",
		Name:    "Google",
		Type:    sso.ProviderTypeOIDC,
		Enabled: true,
	}
}
