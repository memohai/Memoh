package botbackup

import (
	"context"
	"strings"

	memprovider "github.com/memohai/memoh/internal/memory/adapters"
)

// memoryProviderRef captures the resolved memory backend for a bot: its DB id,
// provider type, and (for built-in) the memory_mode. It lets the backup decide
// whether memory is file-backed (rides the workspace /data tree) or external
// (must be exported/imported through the provider's own API).
type memoryProviderRef struct {
	id           string
	providerType string
	mode         string
}

// isFileBackedMemoryType reports whether a provider keeps its canonical memory
// as files under the workspace /data tree. Those backends (built-in, Mem0) are
// captured by the workspace section and rebuilt from files after import, so the
// dedicated memory section is reserved for external services (e.g. OpenViking).
func isFileBackedMemoryType(providerType string) bool {
	switch memprovider.ProviderType(strings.TrimSpace(providerType)) {
	case memprovider.ProviderBuiltin, memprovider.ProviderMem0, "":
		return true
	default:
		return false
	}
}

// resolveMemoryProvider returns the memory backend a bot currently uses. An
// unset provider falls back to the built-in default (file-backed, off mode).
func (s *Service) resolveMemoryProvider(ctx context.Context, botID string) memoryProviderRef {
	ref := memoryProviderRef{providerType: string(memprovider.ProviderBuiltin)}
	if s.settings == nil {
		return ref
	}
	cfg, err := s.settings.GetBot(ctx, botID)
	if err != nil {
		return ref
	}
	providerID := strings.TrimSpace(cfg.MemoryProviderID)
	if providerID == "" || s.memoryProviders == nil {
		return ref
	}
	resp, err := s.memoryProviders.Get(ctx, providerID)
	if err != nil {
		return ref
	}
	ref.id = providerID
	ref.providerType = resp.Provider
	ref.mode = strings.TrimSpace(memprovider.StringFromConfig(resp.Config, "memory_mode"))
	return ref
}

// readExternalMemory pulls every memory entry for a bot from an external
// provider via its GetAll API. File-backed providers return no entries here.
func (s *Service) readExternalMemory(ctx context.Context, botID string, ref memoryProviderRef) ([]memprovider.MemoryItem, error) {
	if s.memoryRegistry == nil || ref.id == "" {
		return nil, nil
	}
	p, err := s.memoryRegistry.Get(ref.id)
	if err != nil {
		return nil, err
	}
	resp, err := p.GetAll(ctx, memprovider.GetAllRequest{BotID: botID, NoStats: true})
	if err != nil {
		return nil, err
	}
	return resp.Results, nil
}

// writeMemory exports long-term memory for external providers as a
// backend-agnostic entries.json. Built-in/Mem0 memory is intentionally skipped:
// it lives as files under /data and is captured by the workspace section.
func (s *Service) writeMemory(ctx context.Context, botID string, writer *zipBackupWriter, opts ExportOptions) error {
	ref := s.resolveMemoryProvider(ctx, botID)
	if isFileBackedMemoryType(ref.providerType) {
		return nil
	}
	items, err := s.readExternalMemory(ctx, botID, ref)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	return writer.writeJSON(memoryEntriesPath, "memory_entries", items, opts)
}

// summarizeMemory reports the exportable memory entry count for the export
// dialog. File-backed memory reports 0 because it ships inside the workspace
// section rather than as standalone entries.
func (s *Service) summarizeMemory(ctx context.Context, botID string) SectionSummary {
	out := SectionSummary{Key: SectionMemory}
	ref := s.resolveMemoryProvider(ctx, botID)
	if isFileBackedMemoryType(ref.providerType) {
		return out
	}
	items, err := s.readExternalMemory(ctx, botID, ref)
	if err != nil {
		return out
	}
	out.Count = len(items)
	out.Items = memoryEntryLabels(items, sectionItemLimit)
	return out
}

// targetMemoryCount returns how many memory entries the target bot already has,
// for overwrite-mode conflict detection. Best-effort: 0 when unknown.
func (s *Service) targetMemoryCount(ctx context.Context, botID string) int {
	ref := s.resolveMemoryProvider(ctx, botID)
	if isFileBackedMemoryType(ref.providerType) {
		return 0
	}
	items, err := s.readExternalMemory(ctx, botID, ref)
	if err != nil {
		return 0
	}
	return len(items)
}

// restoreMemory re-ingests external memory entries into the target bot's
// provider. It only applies to external backends; if the target bot resolves to
// a file-backed provider (or none), the entries are skipped with a warning since
// they cannot be meaningfully reinserted there.
func (s *Service) restoreMemory(ctx context.Context, targetBotID string, state *importState) error {
	entry, ok := state.entries[memoryEntriesPath]
	if !ok || len(entry.data) == 0 {
		return nil
	}
	var items []memprovider.MemoryItem
	if err := unmarshalJSON(entry.data, &items); err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	ref := s.resolveMemoryProvider(ctx, targetBotID)
	if s.memoryRegistry == nil || ref.id == "" || isFileBackedMemoryType(ref.providerType) {
		state.warnings = append(state.warnings, "memory entries skipped: target bot has no external memory provider")
		return nil
	}
	p, err := s.memoryRegistry.Get(ref.id)
	if err != nil {
		state.warnings = append(state.warnings, "memory entries skipped: "+err.Error())
		return nil
	}
	infer := false
	restored := 0
	for _, item := range items {
		text := strings.TrimSpace(item.Memory)
		if text == "" {
			continue
		}
		if _, addErr := p.Add(ctx, memprovider.AddRequest{
			Message: text,
			BotID:   targetBotID,
			Infer:   &infer,
		}); addErr != nil {
			if err := state.itemErr("memory entry", addErr); err != nil {
				return err
			}
			continue
		}
		restored++
	}
	state.counts[SectionMemory] = restored
	return nil
}

// memoryNeedsRebuild reports whether the imported memory left a derived index
// (built-in sparse/dense Qdrant or Mem0 SaaS) stale. That happens when the
// file-backed memory arrived via the workspace section but its index has not
// been rebuilt yet. External providers persist on write and never need this;
// built-in "off" mode keeps no index at all.
func (s *Service) memoryNeedsRebuild(ctx context.Context, botID string, state *importState) bool {
	if state.counts[SectionWorkspace] <= 0 {
		return false
	}
	ref := s.resolveMemoryProvider(ctx, botID)
	if !isFileBackedMemoryType(ref.providerType) {
		return false
	}
	if memprovider.ProviderType(ref.providerType) == memprovider.ProviderBuiltin {
		mode := strings.ToLower(ref.mode)
		if mode == "" || mode == "off" {
			return false
		}
	}
	return true
}

func memoryEntryLabels(items []memprovider.MemoryItem, limit int) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		text := strings.TrimSpace(item.Memory)
		if text == "" {
			continue
		}
		out = append(out, text)
		if len(out) >= limit {
			break
		}
	}
	return out
}
