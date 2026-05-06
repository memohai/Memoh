package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefreshTokenFromContext(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	secret := "test-secret"
	userID := "user-123"

	// Create an initial token with a 5-minute lifespan
	initialDuration := 5 * time.Minute
	initialTokenStr, _, err := GenerateToken(userID, secret, initialDuration)
	require.NoError(t, err)

	// Parse the token to place it into the echo context
	token, err := jwt.Parse(initialTokenStr, func(_ *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	require.NoError(t, err)
	c.Set("user", token)

	// Simulate some time passing to ensure the new token has a different 'iat' and 'exp'
	time.Sleep(1 * time.Second)

	// Run the refresh function
	defaultDuration := 1 * time.Hour
	newTokenStr, newExpiresAt, err := RefreshTokenFromContext(c, secret, defaultDuration)
	require.NoError(t, err)
	assert.NotEmpty(t, newTokenStr)

	// Parse the original token claims for comparison
	originalClaims, ok := token.Claims.(jwt.MapClaims)
	assert.True(t, ok)
	origIat := int64(originalClaims["iat"].(float64))

	// Parse the new token
	newToken, err := jwt.Parse(newTokenStr, func(_ *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	require.NoError(t, err)
	assert.True(t, newToken.Valid)

	newClaims, ok := newToken.Claims.(jwt.MapClaims)
	assert.True(t, ok)

	// Ensure standard payload claims are retained
	assert.Equal(t, userID, newClaims[claimSubject])
	assert.Equal(t, userID, newClaims[claimUserID])

	// Validate the new time bounds
	newIat := int64(newClaims["iat"].(float64))
	newExp := int64(newClaims["exp"].(float64))

	// 1. Ensure time has advanced
	assert.Greater(t, newIat, origIat)

	// 2. Ensure the refreshed token has a positive lifetime and does not exceed the configured default duration
	lifetimeSeconds := newExp - newIat
	assert.Positive(t, lifetimeSeconds)
	assert.LessOrEqual(t, lifetimeSeconds, int64(defaultDuration.Seconds()))

	// 3. Ensure the return value matches the claim
	assert.Equal(t, newExp, newExpiresAt.Unix())
}

func TestRefreshTokenFromContext_MissingUser(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	secret := "test-secret"
	defaultDuration := 1 * time.Hour

	// Context without the "user" key
	_, _, err := RefreshTokenFromContext(c, secret, defaultDuration)
	require.Error(t, err)

	httpErr := &echo.HTTPError{}
	ok := errors.As(err, &httpErr)
	assert.True(t, ok)
	assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
	assert.Equal(t, "invalid token", httpErr.Message)
}

func TestTenantIDFromContext(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	token := &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			claimTenantID: "tenant-123",
			claimUserID:   "user-123",
		},
	}
	c.Set("user", token)

	tenantID, err := TenantIDFromContext(c)
	require.NoError(t, err)
	assert.Equal(t, "tenant-123", tenantID)
}

func TestTenantIDFromContextRequiresExplicitClaim(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	token := &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			claimUserID: "user-123",
		},
	}
	c.Set("user", token)

	_, err := TenantIDFromContext(c)
	require.Error(t, err)

	httpErr := &echo.HTTPError{}
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
	assert.Equal(t, "tenant id missing", httpErr.Message)
}

func TestUserIDFromContextRejectsChatRouteToken(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	token := &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			claimType:   chatTokenType,
			claimUserID: "user-123",
		},
	}
	c.Set("user", token)

	_, err := UserIDFromContext(c)
	require.Error(t, err)

	httpErr := &echo.HTTPError{}
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
	assert.Equal(t, "user token required", httpErr.Message)
}

func TestUserIDFromContextRejectsServiceToken(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	token := &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			claimType:   serviceTokenType,
			claimUserID: "user-123",
		},
	}
	c.Set("user", token)

	_, err := UserIDFromContext(c)
	require.Error(t, err)

	httpErr := &echo.HTTPError{}
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
	assert.Equal(t, "user token required", httpErr.Message)
}

func TestTenantIDFromContextRejectsServiceToken(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	token := &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			claimType:     serviceTokenType,
			claimUserID:   "user-123",
			claimTenantID: "tenant-123",
		},
	}
	c.Set("user", token)

	_, err := TenantIDFromContext(c)
	require.Error(t, err)

	httpErr := &echo.HTTPError{}
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
	assert.Equal(t, "user token required", httpErr.Message)
}

func TestTokenTypeFromContext(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	token := &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			claimType:   "chat_route",
			claimUserID: "user-123",
		},
	}
	c.Set("user", token)

	tokenType, err := TokenTypeFromContext(c)
	require.NoError(t, err)
	assert.Equal(t, "chat_route", tokenType)
}

func TestTokenTypeFromContextMissingTypeReturnsEmpty(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	token := &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			claimUserID: "user-123",
		},
	}
	c.Set("user", token)

	tokenType, err := TokenTypeFromContext(c)
	require.NoError(t, err)
	assert.Empty(t, tokenType)
}

func TestGenerateServiceTokenSetsTokenType(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	tokenStr, _, err := GenerateServiceToken("user-123", "test-secret", time.Minute)
	require.NoError(t, err)

	token, err := jwt.Parse(tokenStr, func(_ *jwt.Token) (interface{}, error) {
		return []byte("test-secret"), nil
	})
	require.NoError(t, err)
	c.Set("user", token)

	tokenType, err := TokenTypeFromContext(c)
	require.NoError(t, err)
	assert.Equal(t, serviceTokenType, tokenType)

	claims, ok := token.Claims.(jwt.MapClaims)
	require.True(t, ok)
	assert.Equal(t, "user-123", claims[claimTenantID])
}

func TestGenerateTokenSetsTenantClaim(t *testing.T) {
	tokenStr, _, err := GenerateToken("user-123", "test-secret", time.Minute)
	require.NoError(t, err)

	token, err := jwt.Parse(tokenStr, func(_ *jwt.Token) (interface{}, error) {
		return []byte("test-secret"), nil
	})
	require.NoError(t, err)

	claims, ok := token.Claims.(jwt.MapClaims)
	require.True(t, ok)
	assert.Equal(t, "user-123", claims[claimTenantID])
}

func TestRefreshTokenFromContextBackfillsTenantClaim(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	token := &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			claimUserID: "user-123",
			"iat":       float64(time.Now().Add(-time.Minute).Unix()),
			"exp":       float64(time.Now().Add(time.Minute).Unix()),
		},
	}
	c.Set("user", token)

	newTokenStr, _, err := RefreshTokenFromContext(c, "test-secret", time.Hour)
	require.NoError(t, err)

	newToken, err := jwt.Parse(newTokenStr, func(_ *jwt.Token) (interface{}, error) {
		return []byte("test-secret"), nil
	})
	require.NoError(t, err)

	claims, ok := newToken.Claims.(jwt.MapClaims)
	require.True(t, ok)
	assert.Equal(t, "user-123", claims[claimTenantID])
}

func TestRefreshTokenFromContextRejectsTypedToken(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	token := &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			claimType:   serviceTokenType,
			claimUserID: "user-123",
			"iat":       float64(time.Now().Add(-time.Minute).Unix()),
			"exp":       float64(time.Now().Add(time.Minute).Unix()),
		},
	}
	c.Set("user", token)

	_, _, err := RefreshTokenFromContext(c, "test-secret", time.Hour)
	require.Error(t, err)

	httpErr := &echo.HTTPError{}
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
	assert.Equal(t, "user token required", httpErr.Message)
}
