package command

import (
	"fmt"
	"sort"
	"strings"

	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/settings"
)

const (
	modelPickerPageSize    = 8
	providerPickerPageSize = 10
)

// modelCandidate is one chat model in the canonical, selection-independent order
// used by the picker. Its position in the canonical slice is the flat index that
// selection callbacks round-trip.
type modelCandidate struct {
	dbID       string
	modelID    string
	name       string
	providerID string
	provider   string
}

type providerGroup struct {
	name     string
	modelIdx []int // indices into the canonical candidate slice
}

// buildModelCandidates returns all chat models in a stable order (provider name,
// then model name) so flat indices are reproducible across renders regardless of
// the current selection. Provider names are resolved once per provider.
func (h *Handler) buildModelCandidates(cc CommandContext, items []models.GetResponse) []modelCandidate {
	provCache := make(map[string]string)
	resolve := func(id string) string {
		if v, ok := provCache[id]; ok {
			return v
		}
		v := h.resolveProviderName(cc, id)
		provCache[id] = v
		return v
	}
	cands := make([]modelCandidate, 0, len(items))
	for _, it := range items {
		cands = append(cands, modelCandidate{
			dbID:       it.ID,
			modelID:    it.ModelID,
			name:       it.Name,
			providerID: it.ProviderID,
			provider:   resolve(it.ProviderID),
		})
	}
	sort.SliceStable(cands, func(i, j int) bool {
		pi, pj := strings.ToLower(cands[i].provider), strings.ToLower(cands[j].provider)
		if pi != pj {
			return pi < pj
		}
		return strings.ToLower(cands[i].name) < strings.ToLower(cands[j].name)
	})
	return cands
}

// groupCandidatesByProvider buckets the (provider-sorted) candidates into
// contiguous, alphabetical provider groups.
func groupCandidatesByProvider(cands []modelCandidate) []providerGroup {
	groups := make([]providerGroup, 0)
	idxByName := make(map[string]int)
	for i, c := range cands {
		gi, ok := idxByName[c.provider]
		if !ok {
			gi = len(groups)
			groups = append(groups, providerGroup{name: c.provider})
			idxByName[c.provider] = gi
		}
		groups[gi].modelIdx = append(groups[gi].modelIdx, i)
	}
	return groups
}

// modelCandidateByFlat re-derives the canonical candidate list and returns the
// candidate at the given flat index. ok is false when the index is out of range
// (e.g. the model set changed between render and tap).
func (h *Handler) modelCandidateByFlat(cc CommandContext, flat int) (modelCandidate, bool, error) {
	items, err := h.modelsService.ListByType(cc.Ctx, models.ModelTypeChat)
	if err != nil {
		return modelCandidate{}, false, err
	}
	cands := h.buildModelCandidates(cc, items)
	if flat < 0 || flat >= len(cands) {
		return modelCandidate{}, false, nil
	}
	return cands[flat], true, nil
}

// buildModelPickerResult produces the model list as a Result: complete flat text
// (for channels without buttons, preserving prior behavior) plus a two-level
// ModelPickerView for interactive channels. Provider grid is shown unless a
// provider is selected (via --prov or a positional provider arg).
func (h *Handler) buildModelPickerResult(cc CommandContext) (*Result, error) {
	items, err := h.modelsService.ListByType(cc.Ctx, models.ModelTypeChat)
	if err != nil {
		return nil, err
	}

	filterProvider := ""
	if len(cc.Args) > 0 {
		filterProvider = strings.TrimSpace(strings.Join(cc.Args, " "))
	}

	settingsResp, _ := h.getBotSettings(cc)

	// Text fallback: flat list, selected-first, preserving prior /model list output.
	textModels := h.filterModelsByProvider(cc, items, filterProvider)
	if len(textModels) == 0 {
		if filterProvider != "" {
			return &Result{Text: fmt.Sprintf("No chat models under provider %q. Run /model list to see all providers.", filterProvider)}, nil
		}
		return &Result{Text: "No chat models yet.\n\nAdd a model in the web dashboard."}, nil
	}
	sort.SliceStable(textModels, func(i, j int) bool {
		return modelSortRank(textModels[i], settingsResp) < modelSortRank(textModels[j], settingsResp)
	})
	records := make([]listRecord, 0, len(textModels))
	for _, item := range textModels {
		fields := []kv{
			{"Model", item.Name},
			{"Provider", h.resolveProviderName(cc, item.ProviderID)},
		}
		// Active-role markers (chat/heartbeat) are a chip, not bracketed into the
		// name — brackets would force the whole name into a monospace code span.
		if markers := modelMarkers(item.ID, settingsResp); len(markers) > 0 {
			fields = append(fields, kv{"", strings.Join(markers, ", ")})
		}
		records = append(records, listRecord{fields: fields})
	}
	hint := "Use /model current to inspect active selections."
	if filterProvider == "" {
		hint = "Use /model list <provider_name> to narrow results."
	}
	res := buildListResult("Chat Models", "model", "list", nil, records, cc.Page, defaultListLimit, hint)

	// Interactive picker. Resolve the drill-down level.
	cands := h.buildModelCandidates(cc, items)
	groups := groupCandidatesByProvider(cands)
	currentDisplay := h.resolveModelName(cc, settingsResp.ChatModelID)
	reasoning := formatReasoningLabel(settingsResp)
	provIdx := cc.Prov
	if provIdx < 0 && filterProvider != "" {
		for i, g := range groups {
			if strings.EqualFold(g.name, filterProvider) {
				provIdx = i
				break
			}
		}
	}
	var picker *ModelPickerView
	if provIdx >= 0 && provIdx < len(groups) {
		picker = buildModelsPickerView(groups, cands, provIdx, settingsResp.ChatModelID, currentDisplay, reasoning, cc.Page)
	} else {
		picker = buildProvidersPickerView(groups, cands, settingsResp.ChatModelID, currentDisplay, reasoning, cc.Page)
	}
	res.Interactive = &Interactive{Kind: InteractiveModelPicker, Picker: picker}
	return res, nil
}

// formatReasoningLabel renders the current reasoning state for picker headers.
func formatReasoningLabel(s settings.Settings) string {
	if !s.ReasoningEnabled {
		return "off"
	}
	if e := strings.TrimSpace(s.ReasoningEffort); e != "" {
		return e
	}
	return "on"
}

func buildProvidersPickerView(groups []providerGroup, cands []modelCandidate, currentDBID, currentDisplay, reasoning string, page int) *ModelPickerView {
	providers := make([]PickerProvider, 0, len(groups))
	for i, g := range groups {
		hasCurrent := false
		for _, mi := range g.modelIdx {
			if cands[mi].dbID == currentDBID {
				hasCurrent = true
				break
			}
		}
		providers = append(providers, PickerProvider{Index: i, Name: g.name, Count: len(g.modelIdx), HasCurrent: hasCurrent})
	}
	return &ModelPickerView{
		Level:            LevelProviders,
		Providers:        providers,
		CurrentModelDBID: currentDBID,
		CurrentDisplay:   currentDisplay,
		Reasoning:        reasoning,
		Page:             page,
		PageSize:         providerPickerPageSize,
		Total:            len(providers),
	}
}

func buildModelsPickerView(groups []providerGroup, cands []modelCandidate, provIdx int, currentDBID, currentDisplay, reasoning string, page int) *ModelPickerView {
	g := groups[provIdx]
	picks := make([]PickerModel, 0, len(g.modelIdx))
	for _, mi := range g.modelIdx {
		c := cands[mi]
		picks = append(picks, PickerModel{
			FlatIndex: mi,
			Name:      c.name,
			Provider:  c.provider,
			Selected:  c.dbID == currentDBID,
		})
	}
	return &ModelPickerView{
		Level:            LevelModels,
		Models:           picks,
		ProviderIndex:    provIdx,
		ProviderName:     g.name,
		CurrentModelDBID: currentDBID,
		CurrentDisplay:   currentDisplay,
		Reasoning:        reasoning,
		Page:             page,
		PageSize:         modelPickerPageSize,
		Total:            len(picks),
	}
}
