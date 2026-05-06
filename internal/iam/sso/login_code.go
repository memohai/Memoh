package sso

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

func GenerateLoginCode() (code string, codeHash string, err error) {
	raw := make([]byte, LoginCodeBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	code = base64.RawURLEncoding.EncodeToString(raw)
	return code, HashLoginCode(code), nil
}

func HashLoginCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}
