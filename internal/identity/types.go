// Package identity provides identity type constants and helpers.
package identity

import "strings"

// Identity type constants: human (user) or bot.
const (
	IdentityTypeHuman = "human"
	IdentityTypeBot   = "bot"
)

// IsBotIdentityType checks if the identity type is a bot.
func IsBotIdentityType(identityType string) bool {
	return strings.EqualFold(strings.TrimSpace(identityType), IdentityTypeBot)
}
