package skills

import (
	"strings"

	"github.com/memohai/memoh/internal/slash"
)

const defaultCatalogScope = "effective"

type SafeCatalogItem struct {
	SkillRef    string `json:"skill_ref"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	SourceKind  string `json:"source_kind"`
	State       string `json:"state"`
}

type ResolvedSkill struct {
	Ref            string
	Name           string
	Description    string
	Content        string
	SourceKind     string
	OpaqueSourceID string
	ContentHash    string
	Identity       string
}

type RequestedRef struct {
	SkillRef string
	Name     string
}

type ResolveLimits struct {
	MaxCount       int
	MaxSingleBytes int
	MaxTotalBytes  int
}

func BuildSafeCatalog(botID string, entries []Entry, codec *RefCodec, catalogScope string) ([]SafeCatalogItem, error) {
	if strings.TrimSpace(catalogScope) == "" {
		catalogScope = defaultCatalogScope
	}
	entries = normalizeRuntimeUsabilityEntries(entries)
	groups := groupEntriesByName(entries)
	out := make([]SafeCatalogItem, 0, len(entries))
	for _, entry := range entries {
		if !isRuntimeUsableCandidate(entry, groups[entry.Name]) {
			continue
		}
		ref, _, err := mintRef(botID, catalogScope, entry, codec)
		if err != nil {
			return nil, err
		}
		out = append(out, SafeCatalogItem{
			SkillRef:    ref,
			Name:        entry.Name,
			DisplayName: entry.Name,
			Description: entry.Description,
			SourceKind:  entry.SourceKind,
			State:       StateEffective,
		})
	}
	return out, nil
}

func ResolveRef(botID string, entries []Entry, codec *RefCodec, catalogScope, name, ref string) (ResolvedSkill, error) {
	if strings.TrimSpace(catalogScope) == "" {
		catalogScope = defaultCatalogScope
	}
	entries = normalizeRuntimeUsabilityEntries(entries)
	decoded, err := codec.DecodeWithKeyID(ref)
	if err != nil {
		return ResolvedSkill{}, err
	}
	payload := decoded.Payload
	if strings.TrimSpace(payload.BotID) != strings.TrimSpace(botID) {
		return ResolvedSkill{}, slash.NewError(slash.CodeInvalidSkillRef)
	}
	if payload.CatalogScope != catalogScope {
		return ResolvedSkill{}, slash.NewError(slash.CodeStaleSkillRef)
	}
	if payload.Name != strings.TrimSpace(name) {
		return ResolvedSkill{}, slash.NewError(slash.CodeInvalidSkillRef)
	}
	groups := groupEntriesByName(entries)
	group := groups[payload.Name]
	if len(group) == 0 {
		return ResolvedSkill{}, slash.NewError(slash.CodeStaleSkillRef)
	}
	for _, entry := range group {
		opaqueSourceID, err := opaqueSourceIDForEntryWithKeyID(codec, decoded.KeyID, entry)
		if err != nil {
			return ResolvedSkill{}, err
		}
		if entry.SourceKind != payload.SourceKind || opaqueSourceID != payload.OpaqueSourceID {
			continue
		}
		contentHash := contentHashForEntry(entry)
		if contentHash != payload.ContentHash {
			return ResolvedSkill{}, slash.NewError(slash.CodeStaleSkillRef)
		}
		if code := runtimeRejectCode(entry, group); code != "" {
			return ResolvedSkill{}, slash.NewError(code)
		}
		return resolvedSkill(ref, entry, opaqueSourceID, contentHash), nil
	}
	return ResolvedSkill{}, slash.NewError(slash.CodeStaleSkillRef)
}

func ResolveRefRequestedSkills(botID string, entries []Entry, requests []RequestedRef, codec *RefCodec, catalogScope string, limits ResolveLimits) ([]ResolvedSkill, error) {
	if strings.TrimSpace(catalogScope) == "" {
		catalogScope = defaultCatalogScope
	}
	out := make([]ResolvedSkill, 0, len(requests))
	seenIdentity := make(map[string]struct{}, len(requests))
	totalBytes := 0
	for _, request := range requests {
		name := strings.TrimSpace(request.Name)
		ref := strings.TrimSpace(request.SkillRef)
		if name == "" || ref == "" {
			return nil, slash.NewError(slash.CodeInvalidSkillRef)
		}
		resolved, err := ResolveRef(botID, entries, codec, catalogScope, name, ref)
		if err != nil {
			return nil, err
		}
		if _, ok := seenIdentity[resolved.Identity]; ok {
			continue
		}
		seenIdentity[resolved.Identity] = struct{}{}
		if limits.MaxCount > 0 && len(out)+1 > limits.MaxCount {
			return nil, slash.NewError(slash.CodeTooManyRequestedSkills)
		}
		contentBytes := len([]byte(resolved.Content))
		totalBytes += contentBytes
		if limits.MaxSingleBytes > 0 && contentBytes > limits.MaxSingleBytes {
			return nil, slash.NewError(slash.CodeRequestedSkillContextTooLarge)
		}
		if limits.MaxTotalBytes > 0 && totalBytes > limits.MaxTotalBytes {
			return nil, slash.NewError(slash.CodeRequestedSkillContextTooLarge)
		}
		out = append(out, resolved)
	}
	return out, nil
}

func ResolveTextRequestedSkills(botID string, entries []Entry, names []string, codec *RefCodec, catalogScope string, limits ResolveLimits) ([]ResolvedSkill, error) {
	if strings.TrimSpace(catalogScope) == "" {
		catalogScope = defaultCatalogScope
	}
	entries = normalizeRuntimeUsabilityEntries(entries)
	deduped := dedupeNames(names)
	if limits.MaxCount > 0 && len(deduped) > limits.MaxCount {
		return nil, slash.NewError(slash.CodeTooManyRequestedSkills)
	}
	groups := groupEntriesByName(entries)
	out := make([]ResolvedSkill, 0, len(deduped))
	totalBytes := 0
	for _, name := range deduped {
		if !IsValidName(name) {
			return nil, slash.NewError(slash.CodeInvalidSkillSlashSyntax)
		}
		group := groups[name]
		if len(group) == 0 {
			return nil, slash.NewError(slash.CodeRequestedSkillNotFound)
		}
		entry, code := resolveTextCandidate(group)
		if code != "" {
			return nil, slash.NewError(code)
		}
		contentBytes := len([]byte(entry.Content))
		totalBytes += contentBytes
		if limits.MaxSingleBytes > 0 && contentBytes > limits.MaxSingleBytes {
			return nil, slash.NewError(slash.CodeRequestedSkillContextTooLarge)
		}
		if limits.MaxTotalBytes > 0 && totalBytes > limits.MaxTotalBytes {
			return nil, slash.NewError(slash.CodeRequestedSkillContextTooLarge)
		}
		ref, payload, err := mintRef(botID, catalogScope, entry, codec)
		if err != nil {
			return nil, err
		}
		out = append(out, resolvedSkill(ref, entry, payload.OpaqueSourceID, payload.ContentHash))
	}
	return out, nil
}

func groupEntriesByName(entries []Entry) map[string][]Entry {
	groups := make(map[string][]Entry, len(entries))
	for _, entry := range entries {
		groups[entry.Name] = append(groups[entry.Name], entry)
	}
	return groups
}

func resolveTextCandidate(group []Entry) (Entry, string) {
	if len(group) == 0 {
		return Entry{}, slash.CodeRequestedSkillNotFound
	}
	if len(group) > 1 {
		return Entry{}, slash.CodeRequestedSkillAmbiguous
	}
	entry := group[0]
	if entry.State == StateDisabled {
		return Entry{}, slash.CodeRequestedSkillDisabled
	}
	if code := runtimeRejectCode(entry, group); code != "" {
		return Entry{}, code
	}
	return entry, ""
}

func runtimeRejectCode(entry Entry, group []Entry) string {
	if len(group) > 1 || entry.State == StateShadowed {
		return slash.CodeRequestedSkillAmbiguous
	}
	if entry.State == StateDisabled {
		return slash.CodeRequestedSkillDisabled
	}
	if !isRuntimeUsableEntry(entry) {
		return slash.CodeRequestedSkillNotRuntimeUsable
	}
	return ""
}

func isRuntimeUsableCandidate(entry Entry, group []Entry) bool {
	return runtimeRejectCode(entry, group) == ""
}

func isRuntimeUsableEntry(entry Entry) bool {
	if !entry.RuntimeUsabilityChecked {
		entry = NormalizeRuntimeUsability(entry)
	}
	return entry.RuntimeUsable
}

func mintRef(botID, catalogScope string, entry Entry, codec *RefCodec) (string, RefPayload, error) {
	opaqueSourceID, err := opaqueSourceIDForEntry(codec, entry)
	if err != nil {
		return "", RefPayload{}, err
	}
	payload := RefPayload{
		BotID:          strings.TrimSpace(botID),
		CatalogScope:   catalogScope,
		Name:           entry.Name,
		SourceKind:     entry.SourceKind,
		OpaqueSourceID: opaqueSourceID,
		ContentHash:    contentHashForEntry(entry),
	}
	ref, err := codec.Encode(payload)
	return ref, payload, err
}

func opaqueSourceIDForEntry(codec *RefCodec, entry Entry) (string, error) {
	return codec.OpaqueSourceID(entry.SourceKind, entry.SourceRoot, entry.SourcePath, entry.Name)
}

func opaqueSourceIDForEntryWithKeyID(codec *RefCodec, kid string, entry Entry) (string, error) {
	return codec.OpaqueSourceIDWithKeyID(kid, entry.SourceKind, entry.SourceRoot, entry.SourcePath, entry.Name)
}

func resolvedSkill(ref string, entry Entry, opaqueSourceID string, contentHash string) ResolvedSkill {
	identity := strings.Join([]string{entry.Name, opaqueSourceID, contentHash}, "|")
	return ResolvedSkill{
		Ref:            ref,
		Name:           entry.Name,
		Description:    entry.Description,
		Content:        entry.Content,
		SourceKind:     entry.SourceKind,
		OpaqueSourceID: opaqueSourceID,
		ContentHash:    contentHash,
		Identity:       identity,
	}
}

func dedupeNames(names []string) []string {
	out := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}
