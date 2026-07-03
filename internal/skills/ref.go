package skills

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/memohai/memoh/internal/slash"
)

const (
	refVersion      = "v1"
	refPurpose      = "skill-ref:v1"
	sourceIDPurpose = "opaque-source-id:v1"
)

type RefPayload struct {
	Version        string `json:"version"`
	BotID          string `json:"bot_id"`
	CatalogScope   string `json:"catalog_scope"`
	Name           string `json:"name"`
	SourceKind     string `json:"source_kind"`
	OpaqueSourceID string `json:"opaque_source_id"`
	ContentHash    string `json:"content_hash"`
}

type DecodedRef struct {
	KeyID   string
	Payload RefPayload
}

type RefCodec struct {
	currentKID string
	keys       map[string][]byte
	rand       io.Reader
}

func NewRefCodec(currentKID string, keys map[string][]byte) (*RefCodec, error) {
	return newRefCodec(currentKID, keys, rand.Reader)
}

func newRefCodec(currentKID string, keys map[string][]byte, random io.Reader) (*RefCodec, error) {
	currentKID = strings.TrimSpace(currentKID)
	if currentKID == "" {
		return nil, errors.New("skill ref current kid is required")
	}
	out := make(map[string][]byte, len(keys))
	for kid, key := range keys {
		kid = strings.TrimSpace(kid)
		if kid == "" {
			continue
		}
		if _, err := aes.NewCipher(key); err != nil {
			return nil, fmt.Errorf("skill ref key %q: %w", kid, err)
		}
		out[kid] = append([]byte(nil), key...)
	}
	if _, ok := out[currentKID]; !ok {
		return nil, fmt.Errorf("skill ref current kid %q not found", currentKID)
	}
	if random == nil {
		random = rand.Reader
	}
	return &RefCodec{currentKID: currentKID, keys: out, rand: random}, nil
}

func (c *RefCodec) Encode(payload RefPayload) (string, error) {
	if c == nil {
		return "", errors.New("skill ref codec is nil")
	}
	key, ok := c.keys[c.currentKID]
	if !ok {
		return "", errors.New("skill ref current key is missing")
	}
	payload.Version = refVersion
	plain, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	aead, err := newAEAD(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(c.rand, nonce); err != nil {
		return "", err
	}
	ad := associatedData(c.currentKID)
	ciphertext := aead.Seal(nil, nonce, plain, ad)
	return strings.Join([]string{
		refVersion,
		c.currentKID,
		base64.RawURLEncoding.EncodeToString(nonce),
		base64.RawURLEncoding.EncodeToString(ciphertext),
	}, "."), nil
}

func (c *RefCodec) Decode(ref string) (RefPayload, error) {
	decoded, err := c.DecodeWithKeyID(ref)
	if err != nil {
		return RefPayload{}, err
	}
	return decoded.Payload, nil
}

func (c *RefCodec) DecodeWithKeyID(ref string) (DecodedRef, error) {
	if c == nil {
		return DecodedRef{}, slash.NewError(slash.CodeInvalidSkillRef)
	}
	ref = strings.TrimSpace(ref)
	parts := strings.Split(ref, ".")
	if len(parts) != 4 || parts[0] != refVersion {
		return DecodedRef{}, slash.NewError(slash.CodeInvalidSkillRef)
	}
	kid := strings.TrimSpace(parts[1])
	key, ok := c.keys[kid]
	if !ok {
		return DecodedRef{}, slash.NewError(slash.CodeInvalidSkillRef)
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return DecodedRef{}, slash.NewError(slash.CodeInvalidSkillRef)
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		return DecodedRef{}, slash.NewError(slash.CodeInvalidSkillRef)
	}
	aead, err := newAEAD(key)
	if err != nil {
		return DecodedRef{}, slash.NewError(slash.CodeInvalidSkillRef)
	}
	if len(nonce) != aead.NonceSize() {
		return DecodedRef{}, slash.NewError(slash.CodeInvalidSkillRef)
	}
	plain, err := aead.Open(nil, nonce, ciphertext, associatedData(kid))
	if err != nil {
		return DecodedRef{}, slash.NewError(slash.CodeInvalidSkillRef)
	}
	var payload RefPayload
	if err := json.Unmarshal(plain, &payload); err != nil {
		return DecodedRef{}, slash.NewError(slash.CodeInvalidSkillRef)
	}
	if payload.Version != refVersion {
		return DecodedRef{}, slash.NewError(slash.CodeInvalidSkillRef)
	}
	return DecodedRef{KeyID: kid, Payload: payload}, nil
}

func (c *RefCodec) OpaqueSourceID(fields ...string) (string, error) {
	return c.OpaqueSourceIDWithKeyID(c.currentKID, fields...)
}

func (c *RefCodec) OpaqueSourceIDWithKeyID(kid string, fields ...string) (string, error) {
	if c == nil {
		return "", errors.New("skill ref codec is nil")
	}
	kid = strings.TrimSpace(kid)
	key, ok := c.keys[kid]
	if !ok {
		return "", fmt.Errorf("skill ref key %q is missing", kid)
	}
	mac := hmac.New(sha256.New, key)
	writeLengthPrefixed(mac, sourceIDPurpose)
	for _, field := range fields {
		writeLengthPrefixed(mac, field)
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func associatedData(kid string) []byte {
	return []byte(refPurpose + ":" + refVersion + ":" + kid)
}

func writeLengthPrefixed(w io.Writer, value string) {
	_, _ = fmt.Fprintf(w, "%d:", len(value))
	_, _ = io.WriteString(w, value)
}

func contentHashForEntry(entry Entry) string {
	content := entry.Raw
	if content == "" {
		content = entry.Content
	}
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
