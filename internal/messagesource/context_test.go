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

func TestNewEnvelopeRequiresCompleteCanonicalOrigin(t *testing.T) {
	t.Parallel()

	complete, err := NewEnvelope(EnvelopeInput{
		Source: V1Candidate{
			SenderDisplayName: "  Alice  ",
			Platform:          "  telegram  ",
			ConversationType:  "  group  ",
			ConversationName:  "  Project Room  ",
		},
	})
	if err != nil {
		t.Fatalf("NewEnvelope() error = %v", err)
	}
	if got := complete.Values().Context; got != NewV1("Alice", "telegram", "group", "Project Room") {
		t.Fatalf("NewEnvelope() context = %+v", got)
	}

	tests := map[string]V1Candidate{
		"missing sender":       {Platform: "telegram", ConversationType: "group", ConversationName: "Room"},
		"missing platform":     {SenderDisplayName: "Alice", ConversationType: "group", ConversationName: "Room"},
		"missing type":         {SenderDisplayName: "Alice", Platform: "telegram", ConversationName: "Room"},
		"missing name":         {SenderDisplayName: "Alice", Platform: "telegram", ConversationType: "group"},
		"noncanonical direct":  {SenderDisplayName: "Alice", Platform: "telegram", ConversationType: "direct", ConversationName: "Room"},
		"unknown conversation": {SenderDisplayName: "Alice", Platform: "telegram", ConversationType: "broadcast", ConversationName: "Room"},
	}
	for name, candidate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			envelope, err := NewEnvelope(EnvelopeInput{Source: candidate})
			if err != nil {
				t.Fatalf("NewEnvelope() error = %v", err)
			}
			if got := envelope.Values().Context; got != (Context{}) {
				t.Fatalf("NewEnvelope() context = %+v, want zero", got)
			}
		})
	}
}

func TestNewEnvelopeValidatesOptionalUUIDsAndKeepsHistoricalCodecSeparate(t *testing.T) {
	t.Parallel()

	if _, err := Encode(NewV1("", "", "", "")); err != nil {
		t.Fatalf("authoritative empty historical V1: %v", err)
	}
	partial, err := NewEnvelope(EnvelopeInput{Source: V1Candidate{SenderDisplayName: "Alice"}})
	if err != nil {
		t.Fatalf("partial live candidate: %v", err)
	}
	if partial.Values().Context != (Context{}) {
		t.Fatalf("partial live context = %+v, want zero", partial.Values().Context)
	}

	valid, err := NewEnvelope(EnvelopeInput{
		SenderChannelIdentityID: " 11111111-1111-1111-1111-111111111111 ",
		SenderUserID:            "22222222-2222-2222-2222-222222222222",
		EventID:                 "33333333-3333-3333-3333-333333333333",
	})
	if err != nil {
		t.Fatalf("valid envelope UUIDs: %v", err)
	}
	if got := valid.Values(); got.SenderChannelIdentityID != "11111111-1111-1111-1111-111111111111" ||
		got.SenderUserID != "22222222-2222-2222-2222-222222222222" ||
		got.EventID != "33333333-3333-3333-3333-333333333333" {
		t.Fatalf("normalized envelope UUIDs = %+v", got)
	}

	for name, input := range map[string]EnvelopeInput{
		"channel identity": {SenderChannelIdentityID: "not-a-uuid"},
		"user":             {SenderUserID: "not-a-uuid"},
		"event":            {EventID: "not-a-uuid"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := NewEnvelope(input); err == nil {
				t.Fatal("invalid optional UUID was accepted")
			}
		})
	}
}
