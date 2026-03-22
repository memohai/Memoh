package weixin

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"testing"
)

func TestEncryptDecryptAESECB(t *testing.T) {
	key := []byte("0123456789abcdef") // 16 bytes
	plaintext := []byte("hello world test")

	ciphertext, err := encryptAESECB(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if bytes.Equal(ciphertext, plaintext) {
		t.Error("ciphertext should differ from plaintext")
	}

	decrypted, err := decryptAESECB(ciphertext, key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted = %q, want %q", string(decrypted), string(plaintext))
	}
}

func TestEncryptDecryptAESECB_ShortInput(t *testing.T) {
	key := []byte("0123456789abcdef")
	plaintext := []byte("hi")

	ciphertext, err := encryptAESECB(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	decrypted, err := decryptAESECB(ciphertext, key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("decrypted = %q, want %q", string(decrypted), string(plaintext))
	}
}

func TestAESECBPaddedSize(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0, 16},
		{1, 16},
		{15, 16},
		{16, 32},
		{17, 32},
	}
	for _, tc := range tests {
		got := aesECBPaddedSize(tc.input)
		if got != tc.want {
			t.Errorf("aesECBPaddedSize(%d) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestParseAESKey_Raw16Bytes(t *testing.T) {
	raw := []byte("0123456789abcdef")
	b64 := base64.StdEncoding.EncodeToString(raw)
	key, err := parseAESKey(b64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(key, raw) {
		t.Errorf("key = %x, want %x", key, raw)
	}
}

func TestParseAESKey_HexEncoded(t *testing.T) {
	rawKey := []byte("0123456789abcdef")
	hexStr := hex.EncodeToString(rawKey) // 32 hex chars
	b64 := base64.StdEncoding.EncodeToString([]byte(hexStr))
	key, err := parseAESKey(b64)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(key, rawKey) {
		t.Errorf("key = %x, want %x", key, rawKey)
	}
}

func TestParseAESKey_Invalid(t *testing.T) {
	b64 := base64.StdEncoding.EncodeToString([]byte("short"))
	_, err := parseAESKey(b64)
	if err == nil {
		t.Error("expected error for invalid key length")
	}
}
