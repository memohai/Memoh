package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestRefreshTokenFromContext(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	secret := "test-secret"
	userID := "user-123"

	// Create an initial token with a 5-minute lifespan
	initialDuration := 5 * time.Minute
	initialTokenStr, _, err := GenerateToken(userID, secret, initialDuration)
	assert.NoError(t, err)

	// Parse the token to place it into the echo context
	token, err := jwt.Parse(initialTokenStr, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	assert.NoError(t, err)
	c.Set("user", token)

	// Simulate some time passing to ensure the new token has a different 'iat' and 'exp'
	time.Sleep(1 * time.Second)

	// Run the refresh function
	defaultDuration := 1 * time.Hour
	newTokenStr, newExpiresAt, err := RefreshTokenFromContext(c, secret, defaultDuration)
	assert.NoError(t, err)
	assert.NotEmpty(t, newTokenStr)

	// Parse the original token claims for comparison
	originalClaims, ok := token.Claims.(jwt.MapClaims)
	assert.True(t, ok)
	origIat := int64(originalClaims["iat"].(float64))

	// Parse the new token
	newToken, err := jwt.Parse(newTokenStr, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	assert.NoError(t, err)
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
	assert.Greater(t, lifetimeSeconds, int64(0))
	assert.LessOrEqual(t, lifetimeSeconds, int64(defaultDuration.Seconds()))

	// 3. Ensure the return value matches the claim
	assert.Equal(t, newExpiresAt.Unix(), newExp)
}

func TestRefreshTokenFromContext_MissingUser(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	secret := "test-secret"
	defaultDuration := 1 * time.Hour

	// Context without the "user" key
	_, _, err := RefreshTokenFromContext(c, secret, defaultDuration)
	assert.Error(t, err)

	httpErr, ok := err.(*echo.HTTPError)
	assert.True(t, ok)
	assert.Equal(t, http.StatusUnauthorized, httpErr.Code)
	assert.Equal(t, "invalid token", httpErr.Message)
}
