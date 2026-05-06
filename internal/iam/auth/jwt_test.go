package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

func TestGenerateTokenIncludesSessionID(t *testing.T) {
	tokenString, expiresAt, err := GenerateToken("user-1", "session-1", "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateToken returned error: %v", err)
	}
	if time.Until(expiresAt) <= 0 {
		t.Fatalf("expiresAt is not in the future: %v", expiresAt)
	}

	claims := parseTestClaims(t, tokenString, "secret")
	if got := claimString(claims, claimSubject); got != "user-1" {
		t.Fatalf("sub = %q, want user-1", got)
	}
	if got := claimString(claims, claimUserID); got != "user-1" {
		t.Fatalf("user_id = %q, want user-1", got)
	}
	if got := claimString(claims, claimSessionID); got != "session-1" {
		t.Fatalf("session_id = %q, want session-1", got)
	}
}

func TestUserIDAndSessionIDFromContext(t *testing.T) {
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	c.Set("user", &jwt.Token{
		Valid: true,
		Claims: jwt.MapClaims{
			claimUserID:    "user-1",
			claimSessionID: "session-1",
		},
	})

	userID, err := UserIDFromContext(c)
	if err != nil {
		t.Fatalf("UserIDFromContext returned error: %v", err)
	}
	if userID != "user-1" {
		t.Fatalf("userID = %q, want user-1", userID)
	}

	sessionID, err := SessionIDFromContext(c)
	if err != nil {
		t.Fatalf("SessionIDFromContext returned error: %v", err)
	}
	if sessionID != "session-1" {
		t.Fatalf("sessionID = %q, want session-1", sessionID)
	}
}

func TestGenerateChatTokenPreservesRouteClaims(t *testing.T) {
	tokenString, _, err := GenerateChatToken(ChatToken{
		BotID:             "bot-1",
		ChatID:            "chat-1",
		RouteID:           "route-1",
		ChannelIdentityID: "channel-identity-1",
	}, "secret", time.Hour)
	if err != nil {
		t.Fatalf("GenerateChatToken returned error: %v", err)
	}
	claims := parseTestClaims(t, tokenString, "secret")
	if got := claimString(claims, claimType); got != chatTokenType {
		t.Fatalf("typ = %q, want %s", got, chatTokenType)
	}

	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
	c.Set("user", &jwt.Token{Valid: true, Claims: claims})
	info, err := ChatTokenFromContext(c)
	if err != nil {
		t.Fatalf("ChatTokenFromContext returned error: %v", err)
	}
	if info.UserID != "channel-identity-1" {
		t.Fatalf("fallback userID = %q, want channel-identity-1", info.UserID)
	}
	if !IsChatTokenContext(c) {
		t.Fatal("IsChatTokenContext returned false")
	}
}

func parseTestClaims(t *testing.T, tokenString, secret string) jwt.MapClaims {
	t.Helper()
	token, err := jwt.Parse(tokenString, func(_ *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		t.Fatalf("jwt.Parse returned error: %v", err)
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		t.Fatal("claims are not jwt.MapClaims")
	}
	return claims
}
