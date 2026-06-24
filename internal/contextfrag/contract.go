package contextfrag

import (
	"errors"
	"fmt"
	"strings"
)

func (ref ContextRef) StableKey() string {
	namespace := strings.TrimSpace(ref.Namespace)
	id := strings.TrimSpace(ref.ID)
	schema := strings.TrimSpace(ref.Schema)
	key := namespace + ":" + id
	if ref.Version != 0 {
		key += fmt.Sprintf("@v%d", ref.Version)
	}
	if ref.Range != nil {
		key += fmt.Sprintf("[%d:%d]", ref.Range.Start, ref.Range.End)
	}
	if schema != "" {
		key += "#" + schema
	}
	return key
}

func (ref ContextRef) EqualIdentity(other ContextRef) bool {
	if strings.TrimSpace(ref.Namespace) != strings.TrimSpace(other.Namespace) {
		return false
	}
	if strings.TrimSpace(ref.ID) != strings.TrimSpace(other.ID) {
		return false
	}
	if ref.Version != other.Version {
		return false
	}
	if strings.TrimSpace(ref.Schema) != strings.TrimSpace(other.Schema) {
		return false
	}
	if ref.Range == nil || other.Range == nil {
		return ref.Range == nil && other.Range == nil
	}
	return ref.Range.Start == other.Range.Start && ref.Range.End == other.Range.End
}

func ValidateContextRef(ref ContextRef) error {
	if strings.TrimSpace(ref.Namespace) == "" {
		return errors.New("context ref namespace is required")
	}
	if strings.TrimSpace(ref.ID) == "" {
		return errors.New("context ref id is required")
	}
	if strings.TrimSpace(ref.Schema) == "" {
		return errors.New("context ref schema is required")
	}
	if err := ValidateSchemaVersions([]SchemaVersion{{Name: ref.Schema, Version: CurrentSchemaVersion}}); err != nil {
		return err
	}
	if ref.Range != nil && ref.Range.End < ref.Range.Start {
		return fmt.Errorf("context ref range end %d before start %d", ref.Range.End, ref.Range.Start)
	}
	if strings.TrimSpace(ref.HashAlgo) != "" && ref.HashAlgo != HashAlgoSHA256 {
		return fmt.Errorf("unsupported context ref hash algo %q", ref.HashAlgo)
	}
	if strings.TrimSpace(ref.HashScope) != "" && ref.HashScope != HashScopeCanonicalFragment {
		return fmt.Errorf("unsupported context ref hash scope %q", ref.HashScope)
	}
	if strings.TrimSpace(ref.ContentHash) != "" {
		if strings.TrimSpace(ref.HashAlgo) == "" {
			return errors.New("context ref hash algo is required when content hash is set")
		}
		if strings.TrimSpace(ref.HashScope) == "" {
			return errors.New("context ref hash scope is required when content hash is set")
		}
	}
	return nil
}

func ValidateSchemaVersions(versions []SchemaVersion) error {
	for _, version := range versions {
		name := strings.TrimSpace(version.Name)
		if name == "" {
			return errors.New("schema version name is required")
		}
		if !knownSchemaName(name) {
			return fmt.Errorf("unknown schema %q", name)
		}
		if version.Version != CurrentSchemaVersion {
			return fmt.Errorf("unsupported schema %q version %d", name, version.Version)
		}
	}
	return nil
}

func (edit ContextEdit) Targets(ref ContextRef) bool {
	for _, got := range edit.Refs {
		if got.EqualIdentity(ref) {
			return true
		}
	}
	return false
}

func CheckEditPreconditions(edit ContextEdit, stateRefs []ContextRef) []ContextConflict {
	var conflicts []ContextConflict
	if edit.Schema.Name != SchemaContextEdit || edit.Schema.Version != CurrentSchemaVersion {
		conflicts = append(conflicts, ContextConflict{
			Kind:     ConflictInvalidSchema,
			Expected: fmt.Sprintf("%s@%d", SchemaContextEdit, CurrentSchemaVersion),
			Actual:   fmt.Sprintf("%s@%d", edit.Schema.Name, edit.Schema.Version),
		})
	} else if err := ValidateSchemaVersions([]SchemaVersion{edit.Schema}); err != nil {
		conflicts = append(conflicts, ContextConflict{
			Kind:     ConflictInvalidSchema,
			Expected: fmt.Sprintf("%s@%d", SchemaContextEdit, CurrentSchemaVersion),
			Actual:   fmt.Sprintf("%s@%d", edit.Schema.Name, edit.Schema.Version),
		})
	}

	byKey := make(map[string]ContextRef, len(stateRefs))
	for _, ref := range stateRefs {
		byKey[ref.StableKey()] = ref
	}

	for key, expected := range edit.Preconditions.ExpectedHashes {
		ref, ok := byKey[key]
		if !ok {
			conflicts = append(conflicts, ContextConflict{Kind: ConflictMissingRef, Key: key, Expected: expected})
			continue
		}
		if ref.ContentHash != expected {
			conflicts = append(conflicts, ContextConflict{
				Kind:     ConflictContentHashMismatch,
				Key:      key,
				Expected: expected,
				Actual:   ref.ContentHash,
			})
		}
	}
	return conflicts
}

func WithContextRef(frag ContextFrag, ref ContextRef) ContextFrag {
	hash, hashErr := CanonicalFragmentHash(frag)
	ref.Namespace = firstNonEmpty(ref.Namespace, frag.Provenance.Source, "context_frag")
	ref.ID = firstNonEmpty(ref.ID, frag.Provenance.SourceID, hash.Value, frag.ID)
	ref.Schema = firstNonEmpty(ref.Schema, SchemaContextRef)
	if hashErr == nil {
		ref.HashAlgo = hash.Algo
		ref.HashScope = hash.Scope
		ref.ContentHash = hash.Value
	}
	frag.Ref = ref
	return frag
}

func DefaultSchemaVersions() []SchemaVersion {
	return []SchemaVersion{
		{Name: SchemaContextManifest, Version: CurrentSchemaVersion},
		{Name: SchemaContextFrag, Version: CurrentSchemaVersion},
		{Name: SchemaContextRef, Version: CurrentSchemaVersion},
		{Name: SchemaContextEdit, Version: CurrentSchemaVersion},
		{Name: SchemaSummaryCoverage, Version: CurrentSchemaVersion},
		{Name: SchemaRenderPolicy, Version: CurrentSchemaVersion},
	}
}

func DefaultSlotRenderPolicies() []SlotRenderPolicy {
	return []SlotRenderPolicy{
		{Slot: SlotSystem, Order: "fragment_order", DedupeBy: "ref", CoverageAware: true, Target: "system"},
		{Slot: SlotBeforeHistory, Order: "fragment_order", DedupeBy: "ref", CoverageAware: true, Target: "messages"},
		{Slot: SlotHistory, Order: "fragment_order", DedupeBy: "ref", CoverageAware: true, Target: "messages"},
		{Slot: SlotAfterHistoryBeforeCurrent, Order: "fragment_order", DedupeBy: "ref", CoverageAware: true, Target: "messages"},
		{Slot: SlotCurrentUser, Order: "last_text_wins_images_append", DedupeBy: "ref", CoverageAware: true, Target: "query_inline_images"},
		{Slot: SlotAfterCurrent, Order: "fragment_order", DedupeBy: "ref", CoverageAware: true, Target: "messages"},
	}
}

func knownSchemaName(name string) bool {
	switch name {
	case SchemaContextManifest, SchemaContextFrag, SchemaContextRef, SchemaContextEdit, SchemaSummaryCoverage, SchemaRenderPolicy:
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
