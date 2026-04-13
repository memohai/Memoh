package providers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	memohcopilot "github.com/memohai/memoh/internal/copilot"
	"github.com/memohai/memoh/internal/db/sqlc"
	"github.com/memohai/memoh/internal/models"
)

const openAIAuthClaimPath = "https://api.openai.com/auth"

type ModelCredentials struct {
	APIKey         string //nolint:gosec // runtime credential material used to construct SDK providers
	CodexAccountID string
}

func SupportsOpenAICodexOAuth(provider sqlc.Provider) bool {
	return supportsOAuth(provider)
}

func (s *Service) ResolveModelCredentials(ctx context.Context, provider sqlc.Provider) (ModelCredentials, error) {
	switch models.ClientType(provider.ClientType) {
	case models.ClientTypeGitHubCopilot:
		githubToken, err := s.GetValidAccessToken(ctx, provider.ID.String())
		if err != nil {
			return ModelCredentials{}, err
		}
		copilotToken, err := memohcopilot.ResolveToken(ctx, githubToken)
		if err != nil {
			return ModelCredentials{}, err
		}
		return ModelCredentials{APIKey: copilotToken}, nil

	case models.ClientTypeOpenAICodex:
		token, err := s.GetValidAccessToken(ctx, provider.ID.String())
		if err != nil {
			return ModelCredentials{}, err
		}
		accountID, err := codexAccountIDFromToken(token)
		if err != nil {
			return ModelCredentials{}, err
		}
		return ModelCredentials{
			APIKey:         token,
			CodexAccountID: accountID,
		}, nil

	default:
		apiKey := ProviderConfigString(provider, "api_key")
		return ModelCredentials{
			APIKey: apiKey,
		}, nil
	}
}

func codexAccountIDFromToken(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", errors.New("invalid oauth access token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode oauth token payload: %w", err)
	}
	var claims struct {
		OpenAIAuth struct {
			ChatGPTAccountID string `json:"chatgpt_account_id"`
		} `json:"https://api.openai.com/auth"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", fmt.Errorf("parse oauth token payload: %w", err)
	}
	accountID := strings.TrimSpace(claims.OpenAIAuth.ChatGPTAccountID)
	if accountID == "" {
		return "", fmt.Errorf("oauth access token missing %s.chatgpt_account_id", openAIAuthClaimPath)
	}
	return accountID, nil
}
