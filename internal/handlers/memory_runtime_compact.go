package handlers

import (
	"context"
	"errors"
	"sort"

	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	"github.com/memohai/memoh/internal/memory/compactplan"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
)

func (r *fileMemoryRuntime) CompactWithLLM(ctx context.Context, filters map[string]any, ratio float64, decayDays int, llm memprovider.LLM) (memprovider.CompactResult, error) {
	botID, err := runtimeBotID("", filters)
	if err != nil {
		return memprovider.CompactResult{}, err
	}
	if ratio <= 0 || ratio > 1 {
		return memprovider.CompactResult{}, errors.New("ratio must be in range (0, 1]")
	}
	items, err := r.store.ReadAllMemoryFiles(ctx, botID)
	if err != nil {
		return memprovider.CompactResult{}, err
	}
	before := len(items)
	if before == 0 {
		return memprovider.CompactResult{BeforeCount: 0, AfterCount: 0, Ratio: ratio, Results: []memprovider.MemoryItem{}}, nil
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt > items[j].UpdatedAt })
	compactedStore, archivedStore, err := compactFileRuntimeItemsWithLLM(ctx, botID, items, ratio, decayDays, llm)
	if err != nil {
		return memprovider.CompactResult{}, err
	}
	if err := r.store.ArchiveAndRebuildFiles(ctx, botID, compactedStore, archivedStore, filters); err != nil {
		return memprovider.CompactResult{}, err
	}
	compacted := runtimeFromStoreItems(compactedStore)
	return memprovider.CompactResult{
		BeforeCount: before,
		AfterCount:  len(compacted),
		Ratio:       ratio,
		Results:     compacted,
	}, nil
}

func compactFileRuntimeItemsWithLLM(ctx context.Context, botID string, items []storefs.MemoryItem, ratio float64, decayDays int, llm memprovider.LLM) ([]storefs.MemoryItem, []storefs.MemoryItem, error) {
	result, err := compactplan.Build(ctx, compactplan.Options{
		BotID:     botID,
		Items:     items,
		Ratio:     ratio,
		DecayDays: decayDays,
		LLM:       llm,
	})
	if err != nil {
		return nil, nil, err
	}
	return result.Active, result.Archived, nil
}
