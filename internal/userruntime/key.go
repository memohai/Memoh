package userruntime

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
)

const (
	KeyPrefix      = "mrk_"
	keyRandomBytes = 32
	keyHexChars    = keyRandomBytes * 2
)

var ErrInvalidKey = errors.New("invalid runtime key")

func NewKey() (string, error) {
	raw := make([]byte, keyRandomBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return KeyPrefix + hex.EncodeToString(raw), nil
}

func ValidateKeyFormat(key string) error {
	key = strings.TrimSpace(key)
	if !strings.HasPrefix(key, KeyPrefix) {
		return ErrInvalidKey
	}
	suffix := strings.TrimPrefix(key, KeyPrefix)
	if len(suffix) != keyHexChars {
		return ErrInvalidKey
	}
	if _, err := hex.DecodeString(suffix); err != nil {
		return ErrInvalidKey
	}
	return nil
}
