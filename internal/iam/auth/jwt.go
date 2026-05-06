package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

const (
	claimSubject           = "sub"
	claimUserID            = "user_id"
	claimSessionID         = "session_id"
	claimChannelIdentityID = "channel_identity_id"
	claimType              = "typ"
	claimBotID             = "bot_id"
	claimChatID            = "chat_id"
	claimRouteID           = "route_id"
	chatTokenType          = "chat_route"
)

func JWTMiddleware(secret string, skipper middleware.Skipper) echo.MiddlewareFunc {
	return echojwt.WithConfig(echojwt.Config{
		SigningKey:    []byte(secret),
		SigningMethod: "HS256",
		TokenLookup:   "header:Authorization:Bearer ,query:token",
		Skipper:       skipper,
		NewClaimsFunc: func(_ echo.Context) jwt.Claims {
			return jwt.MapClaims{}
		},
	})
}

func GenerateToken(userID, sessionID, secret string, expiresIn time.Duration) (string, time.Time, error) {
	if strings.TrimSpace(userID) == "" {
		return "", time.Time{}, errors.New("user id is required")
	}
	if strings.TrimSpace(sessionID) == "" {
		return "", time.Time{}, errors.New("session id is required")
	}
	if strings.TrimSpace(secret) == "" {
		return "", time.Time{}, errors.New("jwt secret is required")
	}
	if expiresIn <= 0 {
		return "", time.Time{}, errors.New("jwt expires in must be positive")
	}

	now := time.Now().UTC()
	expiresAt := now.Add(expiresIn)
	claims := jwt.MapClaims{
		claimSubject:   userID,
		claimUserID:    userID,
		claimSessionID: sessionID,
		"iat":          now.Unix(),
		"exp":          expiresAt.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

func UserIDFromContext(c echo.Context) (string, error) {
	claims, err := claimsFromContext(c)
	if err != nil {
		return "", err
	}
	if userID := claimString(claims, claimUserID); userID != "" {
		return userID, nil
	}
	if userID := claimString(claims, claimSubject); userID != "" {
		return userID, nil
	}
	return "", echo.NewHTTPError(http.StatusUnauthorized, "user id missing")
}

func SessionIDFromContext(c echo.Context) (string, error) {
	claims, err := claimsFromContext(c)
	if err != nil {
		return "", err
	}
	if sessionID := claimString(claims, claimSessionID); sessionID != "" {
		return sessionID, nil
	}
	return "", echo.NewHTTPError(http.StatusUnauthorized, "session id missing")
}

type ChatToken struct {
	BotID             string
	ChatID            string
	RouteID           string
	UserID            string
	ChannelIdentityID string
}

func GenerateChatToken(info ChatToken, secret string, expiresIn time.Duration) (string, time.Time, error) {
	if strings.TrimSpace(info.BotID) == "" {
		return "", time.Time{}, errors.New("bot id is required")
	}
	if strings.TrimSpace(info.ChatID) == "" {
		return "", time.Time{}, errors.New("chat id is required")
	}
	if strings.TrimSpace(info.UserID) == "" {
		info.UserID = strings.TrimSpace(info.ChannelIdentityID)
	}
	if strings.TrimSpace(info.UserID) == "" {
		return "", time.Time{}, errors.New("user id is required")
	}
	if strings.TrimSpace(secret) == "" {
		return "", time.Time{}, errors.New("jwt secret is required")
	}
	if expiresIn <= 0 {
		return "", time.Time{}, errors.New("jwt expires in must be positive")
	}

	now := time.Now().UTC()
	expiresAt := now.Add(expiresIn)
	claims := jwt.MapClaims{
		claimType:              chatTokenType,
		claimBotID:             info.BotID,
		claimChatID:            info.ChatID,
		claimRouteID:           info.RouteID,
		claimUserID:            info.UserID,
		claimChannelIdentityID: info.ChannelIdentityID,
		"iat":                  now.Unix(),
		"exp":                  expiresAt.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

func ChatTokenFromContext(c echo.Context) (ChatToken, error) {
	claims, err := claimsFromContext(c)
	if err != nil {
		return ChatToken{}, err
	}
	if claimString(claims, claimType) != chatTokenType {
		return ChatToken{}, echo.NewHTTPError(http.StatusUnauthorized, "invalid chat token")
	}
	info := ChatToken{
		BotID:             claimString(claims, claimBotID),
		ChatID:            claimString(claims, claimChatID),
		RouteID:           claimString(claims, claimRouteID),
		UserID:            claimString(claims, claimUserID),
		ChannelIdentityID: claimString(claims, claimChannelIdentityID),
	}
	if strings.TrimSpace(info.UserID) == "" {
		info.UserID = strings.TrimSpace(info.ChannelIdentityID)
	}
	return info, nil
}

func IsChatTokenContext(c echo.Context) bool {
	claims, err := claimsFromContext(c)
	if err != nil {
		return false
	}
	return claimString(claims, claimType) == chatTokenType
}

func RefreshTokenFromContext(c echo.Context, secret string, defaultExpiresIn time.Duration) (string, time.Time, error) {
	token, ok := c.Get("user").(*jwt.Token)
	if !ok || token == nil || !token.Valid {
		return "", time.Time{}, echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", time.Time{}, echo.NewHTTPError(http.StatusUnauthorized, "invalid token claims")
	}

	expiresIn := defaultExpiresIn
	if expRaw, ok := claims["exp"].(float64); ok {
		if iatRaw, ok := claims["iat"].(float64); ok {
			duration := time.Duration(expRaw-iatRaw) * time.Second
			if duration > 0 {
				expiresIn = duration
			}
		}
	}

	now := time.Now().UTC()
	expiresAt := now.Add(expiresIn)
	newClaims := jwt.MapClaims{}
	for key, value := range claims {
		newClaims[key] = value
	}
	newClaims["iat"] = now.Unix()
	newClaims["exp"] = expiresAt.Unix()

	newToken := jwt.NewWithClaims(jwt.SigningMethodHS256, newClaims)
	signed, err := newToken.SignedString([]byte(secret))
	if err != nil {
		return "", time.Time{}, err
	}
	return signed, expiresAt, nil
}

func claimsFromContext(c echo.Context) (jwt.MapClaims, error) {
	token, ok := c.Get("user").(*jwt.Token)
	if !ok || token == nil || !token.Valid {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "invalid token")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, echo.NewHTTPError(http.StatusUnauthorized, "invalid token claims")
	}
	return claims, nil
}

func claimString(claims jwt.MapClaims, key string) string {
	raw, ok := claims[key]
	if !ok || raw == nil {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return value
	case fmt.Stringer:
		return value.String()
	default:
		return fmt.Sprint(raw)
	}
}
