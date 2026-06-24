package contextfrag

import (
	"encoding/json"
	"reflect"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"
)

func TestContextRefRoundTripPreservesReducerIdentity(t *testing.T) {
	t.Parallel()

	ref := ContextRef{
		Namespace:   "history",
		ID:          "row-1",
		Version:     2,
		Range:       &ContentRange{Start: 10, End: 20},
		HashAlgo:    HashAlgoSHA256,
		ContentHash: "abc123",
		HashScope:   HashScopeCanonicalFragment,
		Schema:      SchemaContextRef,
	}

	raw, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("marshal context ref: %v", err)
	}
	var got ContextRef
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal context ref: %v", err)
	}

	if !ref.EqualIdentity(got) {
		t.Fatalf("roundtrip ref identity mismatch: got %#v want %#v", got, ref)
	}
	if got.StableKey() != ref.StableKey() {
		t.Fatalf("stable key = %q, want %q", got.StableKey(), ref.StableKey())
	}
	for _, op := range []ContextEditOp{EditReplace, EditRemove, EditCover} {
		edit := ContextEdit{Op: op, Slot: SlotHistory, Refs: []ContextRef{got}}
		if !edit.Targets(ref) {
			t.Fatalf("%s edit should target round-tripped ref", op)
		}
	}
	if err := ValidateContextRef(ContextRef{Namespace: "history", Schema: SchemaContextRef}); err == nil {
		t.Fatal("context ref without ID should fail validation")
	}
}

func TestCanonicalFragmentHashIsStableAndIgnoresDebugID(t *testing.T) {
	t.Parallel()

	frag := TextFrag(TextFragInput{
		ID:         "debug-a",
		Kind:       KindConversationEvent,
		Role:       sdk.MessageRoleUser,
		Slot:       SlotHistory,
		Text:       "hello",
		Priority:   70,
		CacheClass: CacheNever,
		Trust:      TrustExternal,
		Source:     "history",
		SourceID:   "row-1",
		Collector:  "test",
	})
	same := frag
	same.ID = "debug-b"
	same.Provenance.Index = 42
	changed := frag
	changed.Parts = append([]Part(nil), frag.Parts...)
	changed.Parts[0].Text = "hello again"

	hash, err := CanonicalFragmentHash(frag)
	if err != nil {
		t.Fatalf("canonical hash: %v", err)
	}
	sameHash, err := CanonicalFragmentHash(same)
	if err != nil {
		t.Fatalf("canonical hash for same content: %v", err)
	}
	changedHash, err := CanonicalFragmentHash(changed)
	if err != nil {
		t.Fatalf("canonical hash for changed content: %v", err)
	}

	if hash.Algo != HashAlgoSHA256 || hash.Scope != HashScopeCanonicalFragment {
		t.Fatalf("hash metadata = %#v", hash)
	}
	if hash.Value == "" {
		t.Fatal("canonical hash should not be empty")
	}
	if sameHash.Value != hash.Value {
		t.Fatalf("canonical hash should ignore debug ID and positional index: got %q want %q", sameHash.Value, hash.Value)
	}
	if changedHash.Value == hash.Value {
		t.Fatalf("canonical hash should change when fragment content changes: %q", hash.Value)
	}
}

func TestCanonicalFragmentHashDiscriminatesSDKMessagePartTypes(t *testing.T) {
	t.Parallel()

	textFrag := MessageFrag(MessageFragInput{
		ID:      "message.text",
		Message: sdk.Message{Role: sdk.MessageRoleAssistant, Content: []sdk.MessagePart{sdk.TextPart{Text: "same"}}},
		Kind:    KindConversationEvent,
		Slot:    SlotHistory,
	})
	reasoningFrag := MessageFrag(MessageFragInput{
		ID:      "message.reasoning",
		Message: sdk.Message{Role: sdk.MessageRoleAssistant, Content: []sdk.MessagePart{sdk.ReasoningPart{Text: "same"}}},
		Kind:    KindConversationEvent,
		Slot:    SlotHistory,
	})

	textHash, err := CanonicalFragmentHash(textFrag)
	if err != nil {
		t.Fatalf("text hash: %v", err)
	}
	reasoningHash, err := CanonicalFragmentHash(reasoningFrag)
	if err != nil {
		t.Fatalf("reasoning hash: %v", err)
	}
	if textHash.Value == reasoningHash.Value {
		t.Fatalf("text and reasoning SDK message parts must not collide: %q", textHash.Value)
	}
}

func TestCanonicalFragmentHashIncludesNativeImageSDKPayload(t *testing.T) {
	t.Parallel()

	base := ImageFrag("image", []sdk.ImagePart{{
		Image:     "data:image/png;base64,abc",
		MediaType: "image/png",
	}}, Scope{}, "test")
	withCache := ImageFrag("image", []sdk.ImagePart{{
		Image:        "data:image/png;base64,abc",
		MediaType:    "image/png",
		CacheControl: &sdk.CacheControl{Type: "ephemeral"},
	}}, Scope{}, "test")

	baseHash, err := CanonicalFragmentHash(base)
	if err != nil {
		t.Fatalf("base image hash: %v", err)
	}
	cacheHash, err := CanonicalFragmentHash(withCache)
	if err != nil {
		t.Fatalf("cache-control image hash: %v", err)
	}
	if baseHash.Value == cacheHash.Value {
		t.Fatalf("image fragments with different SDK cache control must not collide: %q", baseHash.Value)
	}
}

func TestSerdeRoundTripsContextFragManifestEditAndCoverage(t *testing.T) {
	t.Parallel()

	frag := TextFrag(TextFragInput{
		ID:         "history.001",
		Kind:       KindConversationEvent,
		Role:       sdk.MessageRoleUser,
		Slot:       SlotHistory,
		Text:       "hello",
		Priority:   70,
		CacheClass: CacheNever,
		Trust:      TrustExternal,
		Source:     "history",
		SourceID:   "row-1",
		Collector:  "test",
	})
	frag = WithContextRef(frag, ContextRef{
		Namespace: "history",
		ID:        "row-1",
		Version:   1,
		Schema:    SchemaContextRef,
	})

	roundTripJSON(t, frag, func(got ContextFrag) {
		if !frag.Ref.EqualIdentity(got.Ref) {
			t.Fatalf("frag ref mismatch after roundtrip: got %#v want %#v", got.Ref, frag.Ref)
		}
		if got.Ref.ContentHash == "" {
			t.Fatal("frag ref should carry canonical content hash")
		}
	})
	msgFrag := WithContextRef(MessageFrag(MessageFragInput{
		ID:         "message.001",
		Message:    sdk.UserMessage("from sdk", sdk.ImagePart{Image: "data:image/png;base64,abc", MediaType: "image/png"}),
		Kind:       KindConversationEvent,
		Slot:       SlotHistory,
		Priority:   70,
		CacheClass: CacheNever,
		Trust:      TrustExternal,
		Source:     "history",
		SourceID:   "row-2",
		Collector:  "test",
	}), ContextRef{
		Namespace: "history",
		ID:        "row-2",
		Version:   1,
		Schema:    SchemaContextRef,
	})
	roundTripJSON(t, msgFrag, func(got ContextFrag) {
		rendered := Render([]ContextFrag{got})
		if len(rendered.Messages) != 1 {
			t.Fatalf("round-tripped sdk message fragment rendered %d messages, want 1", len(rendered.Messages))
		}
		if rendered.Messages[0].Role != sdk.MessageRoleUser {
			t.Fatalf("round-tripped sdk message role = %q, want user", rendered.Messages[0].Role)
		}
		if !reflect.DeepEqual(rendered.Messages[0], *msgFrag.Parts[0].Message) {
			t.Fatalf("round-tripped sdk message mismatch: got %#v want %#v", rendered.Messages[0], msgFrag.Parts[0].Message)
		}
	})

	manifest := BuildManifest([]ContextFrag{frag, msgFrag})
	roundTripJSON(t, manifest, func(got Manifest) {
		if !hasSchemaVersion(got.SchemaVersions, SchemaContextManifest, CurrentSchemaVersion) {
			t.Fatalf("manifest schema versions missing manifest version: %#v", got.SchemaVersions)
		}
		if len(got.Items) != 2 || !frag.Ref.EqualIdentity(got.Items[0].Ref) {
			t.Fatalf("manifest item ref mismatch: %#v", got.Items)
		}
		if len(got.SlotPolicies) == 0 {
			t.Fatal("manifest should include per-slot render policies")
		}
		if got.Counts.Images != 1 {
			t.Fatalf("manifest image count = %d, want 1", got.Counts.Images)
		}
	})

	edit := ContextEdit{
		EditID: "edit-1",
		Slot:   SlotHistory,
		Op:     EditReplace,
		Refs:   []ContextRef{frag.Ref},
		Payload: []ContextFrag{
			frag,
		},
		Preconditions: EditPreconditions{
			ExpectedRevision: "rev-1",
			MaxSequence:      42,
			ExpectedHashes:   map[string]string{frag.Ref.StableKey(): frag.Ref.ContentHash},
		},
		Schema: SchemaVersion{Name: SchemaContextEdit, Version: CurrentSchemaVersion},
	}
	roundTripJSON(t, edit, func(got ContextEdit) {
		if !got.Targets(frag.Ref) {
			t.Fatalf("round-tripped edit should target payload ref: %#v", got)
		}
	})

	coverage := SummaryCoverage{
		CoverageID:     "coverage-1",
		SummaryRef:     ContextRef{Namespace: "summary", ID: "summary-1", Schema: SchemaContextRef},
		CoveredRefs:    []ContextRef{frag.Ref},
		CoveredFragIDs: []string{frag.ID},
		Schema:         SchemaVersion{Name: SchemaSummaryCoverage, Version: CurrentSchemaVersion},
	}
	roundTripJSON(t, coverage, func(got SummaryCoverage) {
		if len(got.CoveredRefs) != 1 || !got.CoveredRefs[0].EqualIdentity(frag.Ref) {
			t.Fatalf("coverage refs mismatch after roundtrip: %#v", got.CoveredRefs)
		}
	})

	group := ContinuityGroup{
		ID:               "tool-call-1",
		Kind:             "tool_call",
		Provider:         "openai",
		ModelFamily:      "responses",
		Refs:             []ContextRef{frag.Ref},
		MustKeepTogether: true,
		MustKeepRaw:      true,
		MustKeepOrder:    true,
		MustBeComplete:   true,
	}
	roundTripJSON(t, group, func(got ContinuityGroup) {
		if len(got.Refs) != 1 || !got.Refs[0].EqualIdentity(frag.Ref) || !got.MustKeepTogether || !got.MustBeComplete {
			t.Fatalf("continuity group mismatch after roundtrip: %#v", got)
		}
	})
}

func TestSchemaAndHashValidationRejectsSilentDrift(t *testing.T) {
	t.Parallel()

	if err := ValidateSchemaVersions([]SchemaVersion{{Name: SchemaContextFrag, Version: 999}}); err == nil {
		t.Fatal("unknown context frag schema version should fail validation")
	}
	if err := ValidateSchemaVersions([]SchemaVersion{{Name: "unknown_schema", Version: CurrentSchemaVersion}}); err == nil {
		t.Fatal("unknown schema name should fail validation")
	}
	if err := ValidateContextRef(ContextRef{
		Namespace: "history",
		ID:        "row-1",
		HashScope: "rendered_payload",
		Schema:    SchemaContextRef,
	}); err == nil {
		t.Fatal("non-canonical hash scope should fail even when content hash is empty")
	}
	wrongSchema := ContextEdit{
		EditID: "edit-wrong-schema",
		Slot:   SlotHistory,
		Op:     EditReplace,
		Schema: SchemaVersion{Name: SchemaContextRef, Version: CurrentSchemaVersion},
	}
	schemaConflicts := CheckEditPreconditions(wrongSchema, nil)
	if len(schemaConflicts) != 1 || schemaConflicts[0].Kind != ConflictInvalidSchema {
		t.Fatalf("wrong edit schema should produce invalid-schema conflict, got %#v", schemaConflicts)
	}

	ref := ContextRef{
		Namespace:   "history",
		ID:          "row-1",
		HashAlgo:    HashAlgoSHA256,
		HashScope:   HashScopeCanonicalFragment,
		ContentHash: "good",
		Schema:      SchemaContextRef,
	}
	edit := ContextEdit{
		EditID: "edit-1",
		Slot:   SlotHistory,
		Op:     EditReplace,
		Refs:   []ContextRef{ref},
		Preconditions: EditPreconditions{
			ExpectedHashes: map[string]string{ref.StableKey(): "bad"},
		},
		Schema: SchemaVersion{Name: SchemaContextEdit, Version: CurrentSchemaVersion},
	}
	conflicts := CheckEditPreconditions(edit, []ContextRef{ref})
	if len(conflicts) != 1 || conflicts[0].Kind != ConflictContentHashMismatch {
		t.Fatalf("expected one content-hash conflict, got %#v", conflicts)
	}

	missing := CheckEditPreconditions(edit, nil)
	if len(missing) != 1 || missing[0].Kind != ConflictMissingRef {
		t.Fatalf("expected one missing-ref conflict, got %#v", missing)
	}
}

func TestCompileAddsRefsAndManifestTraceWithoutChangingRenderedLegacyView(t *testing.T) {
	t.Parallel()

	system := "base system"
	messages := []sdk.Message{sdk.UserMessage("history")}
	images := []sdk.ImagePart{{Image: "data:image/png;base64,abc", MediaType: "image/png"}}

	got := Compile(CompileInput{
		Source:       "test",
		System:       system,
		Messages:     messages,
		Query:        "current",
		InlineImages: images,
	})

	if got.System != system || got.Query != "current" || len(got.Messages) != 1 || len(got.InlineImages) != 1 {
		t.Fatalf("rendered legacy view changed: system=%q query=%q messages=%d images=%d", got.System, got.Query, len(got.Messages), len(got.InlineImages))
	}
	for _, frag := range got.Frags {
		if err := ValidateContextRef(frag.Ref); err != nil {
			t.Fatalf("compiled frag %q has invalid ref %#v: %v", frag.ID, frag.Ref, err)
		}
		if frag.Ref.HashScope != HashScopeCanonicalFragment || frag.Ref.ContentHash == "" {
			t.Fatalf("compiled frag %q missing canonical hash ref: %#v", frag.ID, frag.Ref)
		}
	}
	if len(got.Manifest.RenderedOutputs) == 0 {
		t.Fatal("manifest should explain rendered output refs")
	}
	if len(got.Manifest.ValidationWarnings) != 0 {
		t.Fatalf("unexpected manifest validation warnings: %#v", got.Manifest.ValidationWarnings)
	}
}

func TestCompileUsesStableContentAddressedRefsWhenSourceIDIsMissing(t *testing.T) {
	t.Parallel()

	first := Compile(CompileInput{
		Source:   "test",
		Messages: []sdk.Message{sdk.UserMessage("target")},
	})
	second := Compile(CompileInput{
		Source:   "test",
		Messages: []sdk.Message{sdk.UserMessage("prefix"), sdk.UserMessage("target")},
	})

	firstRef, ok := refForRenderedMessageText(first.Frags, "target")
	if !ok {
		t.Fatalf("missing target ref in first compile: %#v", first.Manifest.Items)
	}
	secondRef, ok := refForRenderedMessageText(second.Frags, "target")
	if !ok {
		t.Fatalf("missing target ref in second compile: %#v", second.Manifest.Items)
	}
	if firstRef.ID != secondRef.ID {
		t.Fatalf("source-less legacy message ref drifted with position: first=%#v second=%#v", firstRef, secondRef)
	}
}

func TestManifestRenderedOutputsOnlyIncludeRenderedRefs(t *testing.T) {
	t.Parallel()

	historyText := WithContextRef(TextFrag(TextFragInput{
		ID:   "history.text",
		Kind: KindConversationEvent,
		Slot: SlotHistory,
		Text: "not rendered by legacy renderer",
	}), ContextRef{Namespace: "test", ID: "history.text", Schema: SchemaContextRef})
	currentA := WithContextRef(TextFrag(TextFragInput{
		ID:   "current.a",
		Kind: KindCurrentUserMessage,
		Slot: SlotCurrentUser,
		Text: "a",
	}), ContextRef{Namespace: "test", ID: "current.a", Schema: SchemaContextRef})
	currentB := WithContextRef(TextFrag(TextFragInput{
		ID:   "current.b",
		Kind: KindCurrentUserMessage,
		Slot: SlotCurrentUser,
		Text: "b",
	}), ContextRef{Namespace: "test", ID: "current.b", Schema: SchemaContextRef})

	manifest := BuildManifest([]ContextFrag{historyText, currentA, currentB})
	if renderedOutputContainsRef(manifest.RenderedOutputs, historyText.Ref) {
		t.Fatalf("history text ref should not be listed as rendered output: %#v", manifest.RenderedOutputs)
	}
	if renderedOutputContainsRef(manifest.RenderedOutputs, currentA.Ref) {
		t.Fatalf("overwritten current-user text ref should not be listed as rendered output: %#v", manifest.RenderedOutputs)
	}
	if !renderedOutputContainsRef(manifest.RenderedOutputs, currentB.Ref) {
		t.Fatalf("last current-user text ref should be listed as rendered output: %#v", manifest.RenderedOutputs)
	}
}

func roundTripJSON[T any](t *testing.T, input T, check func(T)) {
	t.Helper()

	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal %T: %v", input, err)
	}
	var got T
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal %T: %v", input, err)
	}
	check(got)
}

func hasSchemaVersion(versions []SchemaVersion, name string, version int) bool {
	for _, got := range versions {
		if got.Name == name && got.Version == version {
			return true
		}
	}
	return false
}

func refForRenderedMessageText(frags []ContextFrag, text string) (ContextRef, bool) {
	for _, frag := range frags {
		for _, part := range frag.Parts {
			msg := partMessage(part)
			if msg == nil {
				continue
			}
			for _, content := range msg.Content {
				textPart, ok := content.(sdk.TextPart)
				if ok && textPart.Text == text {
					return frag.Ref, true
				}
			}
		}
	}
	return ContextRef{}, false
}

func renderedOutputContainsRef(outputs []RenderedOutputRef, ref ContextRef) bool {
	for _, output := range outputs {
		for _, got := range output.Refs {
			if got.EqualIdentity(ref) {
				return true
			}
		}
	}
	return false
}
