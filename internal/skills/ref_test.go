package skills

import (
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/slash"
)

func testRefCodec(t *testing.T) *RefCodec {
	t.Helper()
	key := []byte("0123456789abcdef0123456789abcdef")
	codec, err := newRefCodec("kid1", map[string][]byte{"kid1": key}, &deterministicReader{})
	if err != nil {
		t.Fatalf("newRefCodec: %v", err)
	}
	return codec
}

type deterministicReader struct {
	next byte
}

func (r *deterministicReader) Read(p []byte) (int, error) {
	for i := range p {
		r.next++
		p[i] = r.next
	}
	return len(p), nil
}

func TestRefCodecRoundTripOpaque(t *testing.T) {
	codec := testRefCodec(t)
	ref, err := codec.Encode(RefPayload{
		BotID:          "bot-1",
		CatalogScope:   "effective",
		Name:           "alpha",
		SourceKind:     SourceKindManaged,
		OpaqueSourceID: "source-1",
		ContentHash:    "hash-1",
	})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if strings.Contains(ref, "bot-1") || strings.Contains(ref, "alpha") || strings.Contains(ref, "hash-1") {
		t.Fatalf("ref leaks payload: %q", ref)
	}
	payload, err := codec.Decode(ref)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if payload.BotID != "bot-1" || payload.Name != "alpha" || payload.ContentHash != "hash-1" {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestRefCodecTamperInvalid(t *testing.T) {
	codec := testRefCodec(t)
	ref, err := codec.Encode(RefPayload{BotID: "bot-1", CatalogScope: "effective", Name: "alpha"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	ref = ref[:len(ref)-1] + "x"
	_, err = codec.Decode(ref)
	assertSlashCode(t, err, slash.CodeInvalidSkillRef)
}

func TestRefCodecWrongNonceLengthInvalid(t *testing.T) {
	codec := testRefCodec(t)
	ref, err := codec.Encode(RefPayload{BotID: "bot-1", CatalogScope: "effective", Name: "alpha"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	parts := strings.Split(ref, ".")
	if len(parts) != 4 {
		t.Fatalf("ref parts = %d, want 4", len(parts))
	}
	parts[2] = "AA"
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Decode panicked for malformed nonce: %v", recovered)
		}
	}()
	_, err = codec.Decode(strings.Join(parts, "."))
	assertSlashCode(t, err, slash.CodeInvalidSkillRef)
}

func TestRefCodecUnknownKidInvalid(t *testing.T) {
	codec := testRefCodec(t)
	ref, err := codec.Encode(RefPayload{BotID: "bot-1", CatalogScope: "effective", Name: "alpha"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	ref = strings.Replace(ref, ".kid1.", ".kid2.", 1)
	_, err = codec.Decode(ref)
	assertSlashCode(t, err, slash.CodeInvalidSkillRef)
}
