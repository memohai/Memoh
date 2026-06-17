package builtin

import (
	"context"

	adapters "github.com/memohai/memoh/internal/memory/adapters"
	"github.com/memohai/memoh/internal/memory/compactplan"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
)

const compactMaxCandidateChars = compactplan.MaxCandidateChars

func compactStoreItemsWithLLM(ctx context.Context, botID string, items []storefs.MemoryItem, ratio float64, decayDays int, llm adapters.LLM) ([]storefs.MemoryItem, []storefs.MemoryItem, error) {
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

func compactCandidateChars(candidates []adapters.CandidateMemory) int {
	return compactplan.CandidateChars(candidates)
}
