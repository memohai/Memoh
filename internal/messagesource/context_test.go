package messagesource

import (
	"testing"
)

func TestV1CodecCanonicalizesSourceContext(t *testing.T) {
	t.Parallel()

	want := NewV1(" Alice ", " telegram ", " group ", " Ops Room ")
	encoded, err := Encode(want)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	const wantJSON = `{"version":1,"sender_display_name":"Alice","platform":"telegram","conversation_type":"group","conversation_name":"Ops Room"}`
	if string(encoded) != wantJSON {
		t.Fatalf("Encode() = %s, want %s", encoded, wantJSON)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if decoded != want {
		t.Fatalf("Decode() = %+v, want %+v", decoded, want)
	}
}

func TestDecodeAcceptsAbsentSourceContext(t *testing.T) {
	t.Parallel()

	for _, raw := range [][]byte{nil, {}, []byte("  "), []byte("null")} {
		got, err := Decode(raw)
		if err != nil {
			t.Fatalf("Decode(%q) error = %v", raw, err)
		}
		if got != (Context{}) {
			t.Fatalf("Decode(%q) = %+v, want zero", raw, got)
		}
	}
}

func TestV1CodecRejectsInvalidWireValues(t *testing.T) {
	t.Parallel()

	invalid := []string{
		`{"version":2,"sender_display_name":"Alice","platform":"telegram","conversation_type":"group","conversation_name":"Ops Room"}`,
		`{"version":1.0,"sender_display_name":"Alice","platform":"telegram","conversation_type":"group","conversation_name":"Ops Room"}`,
		`{"version":"1","sender_display_name":"Alice","platform":"telegram","conversation_type":"group","conversation_name":"Ops Room"}`,
		`{"version":1,"sender_display_name":"Alice","platform":"telegram","conversation_type":"group"}`,
		`{"version":1,"sender_display_name":1,"platform":"telegram","conversation_type":"group","conversation_name":"Ops Room"}`,
		`{"version":1,"sender_display_name":"Alice","platform":"telegram","conversation_type":"group","conversation_name":"Ops Room","extra":true}`,
		`{"version":1,"sender_display_name":"Alice","platform":"telegram","conversation_type":"group","conversation_name":"Ops Room"} {}`,
	}
	for _, raw := range invalid {
		if got, err := Decode([]byte(raw)); err == nil {
			t.Fatalf("Decode(%s) = %+v, want error", raw, got)
		}
	}
	for _, context := range []Context{{}, {Version: VersionInvalid}, {Version: 2}} {
		if got, err := Encode(context); err == nil {
			t.Fatalf("Encode(%+v) = %s, want error", context, got)
		}
	}
}
