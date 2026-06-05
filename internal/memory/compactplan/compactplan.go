package compactplan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	adapters "github.com/memohai/memoh/internal/memory/adapters"
	storefs "github.com/memohai/memoh/internal/memory/storefs"
)

const MaxCandidateChars = 24000

type Options struct {
	BotID     string
	Items     []storefs.MemoryItem
	Ratio     float64
	DecayDays int
	LLM       adapters.LLM
}

type Result struct {
	Active   []storefs.MemoryItem
	Archived []storefs.MemoryItem
}

func Build(ctx context.Context, opts Options) (Result, error) {
	opts.BotID = strings.TrimSpace(opts.BotID)
	if opts.BotID == "" {
		return Result{}, errors.New("memory compact requires bot id")
	}
	if opts.LLM == nil {
		return Result{}, errors.New("memory compact requires an LLM")
	}
	protected, compactable := splitProtected(opts.Items)
	target := targetCount(len(compactable), opts.Ratio)
	candidates := make([]adapters.CandidateMemory, 0, len(compactable))
	sourceIDs := make([]string, 0, len(compactable))
	for _, item := range compactable {
		item = canonical(item)
		if item.ID == "" || item.Memory == "" {
			continue
		}
		sourceIDs = append(sourceIDs, item.ID)
		candidates = append(candidates, adapters.CandidateMemory{
			ID:        item.ID,
			Memory:    item.Memory,
			CreatedAt: item.CreatedAt,
			Metadata:  item.Metadata,
		})
	}
	if len(candidates) == 0 {
		return Result{Active: protected}, nil
	}

	facts, err := candidateFacts(ctx, opts.BotID, candidates, target, opts.DecayDays, opts.LLM)
	if err != nil {
		return Result{}, err
	}
	if len(facts) == 0 {
		return Result{}, errors.New("memory compact produced no facts")
	}

	now := time.Now().UTC()
	compacted := make([]storefs.MemoryItem, 0, len(facts))
	compactedIDs := make([]string, 0, len(facts))
	for i, fact := range facts {
		itemTime := now.Add(time.Duration(i))
		id := memoryID(opts.BotID, itemTime)
		ts := itemTime.Format(time.RFC3339)
		compactedIDs = append(compactedIDs, id)
		compacted = append(compacted, storefs.MemoryItem{
			ID:        id,
			Memory:    fact,
			Hash:      memoryHash(fact),
			BotID:     opts.BotID,
			CreatedAt: ts,
			UpdatedAt: ts,
			Metadata: map[string]any{
				"compacted_at":            now.Format(time.RFC3339),
				"compaction_source_ids":   sourceIDs,
				"compaction_target_count": target,
			},
		})
	}

	active := make([]storefs.MemoryItem, 0, len(protected)+len(compacted))
	active = append(active, protected...)
	active = append(active, compacted...)
	return Result{
		Active:   active,
		Archived: archiveItems(compactable, now, compactedIDs),
	}, nil
}

func CandidateChars(candidates []adapters.CandidateMemory) int {
	total := 0
	for _, candidate := range candidates {
		total += len(candidate.ID) + len(candidate.Memory) + len(candidate.CreatedAt) + 32
		for key, value := range candidate.Metadata {
			total += len(key) + len(fmt.Sprint(value)) + 8
		}
	}
	return total
}

func splitProtected(items []storefs.MemoryItem) ([]storefs.MemoryItem, []storefs.MemoryItem) {
	protected := make([]storefs.MemoryItem, 0, len(items))
	compactable := make([]storefs.MemoryItem, 0, len(items))
	for _, item := range items {
		if protectedItem(item) {
			protected = append(protected, item)
			continue
		}
		compactable = append(compactable, item)
	}
	return protected, compactable
}

func protectedItem(item storefs.MemoryItem) bool {
	return metadataTruthy(item.Metadata, "pinned") || metadataTruthy(item.Metadata, "read_only")
}

func metadataTruthy(metadata map[string]any, key string) bool {
	value, ok := metadata[key]
	if !ok {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "y", "on":
			return true
		default:
			return false
		}
	default:
		return strings.EqualFold(strings.TrimSpace(fmt.Sprint(v)), "true")
	}
}

func candidateFacts(ctx context.Context, botID string, candidates []adapters.CandidateMemory, target int, decayDays int, llm adapters.LLM) ([]string, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	if target < 1 {
		target = 1
	}
	if CandidateChars(candidates) <= MaxCandidateChars || len(candidates) == 1 {
		resp, err := llm.Compact(ctx, adapters.CompactRequest{
			BotID:       botID,
			Memories:    candidates,
			TargetCount: target,
			DecayDays:   decayDays,
		})
		if err != nil {
			return nil, err
		}
		return facts(resp.Facts, target), nil
	}

	batches := candidateBatches(candidates)
	collected := make([]string, 0, target)
	for _, batch := range batches {
		batchTarget := batchTarget(target, len(batch), len(candidates))
		resp, err := llm.Compact(ctx, adapters.CompactRequest{
			BotID:       botID,
			Memories:    batch,
			TargetCount: batchTarget,
			DecayDays:   decayDays,
		})
		if err != nil {
			return nil, err
		}
		collected = append(collected, facts(resp.Facts, batchTarget)...)
	}
	collected = facts(collected, 0)
	if len(collected) <= target {
		return collected, nil
	}

	reduceCandidates := make([]adapters.CandidateMemory, 0, len(collected))
	for i, fact := range collected {
		reduceCandidates = append(reduceCandidates, adapters.CandidateMemory{
			ID:     fmt.Sprintf("compact_batch:%d", i),
			Memory: fact,
		})
	}
	return candidateFacts(ctx, botID, reduceCandidates, target, decayDays, llm)
}

func candidateBatches(candidates []adapters.CandidateMemory) [][]adapters.CandidateMemory {
	batches := make([][]adapters.CandidateMemory, 0, 1)
	current := make([]adapters.CandidateMemory, 0)
	currentChars := 0
	for _, candidate := range candidates {
		candidateChars := CandidateChars([]adapters.CandidateMemory{candidate})
		if len(current) > 0 && currentChars+candidateChars > MaxCandidateChars {
			batches = append(batches, current)
			current = make([]adapters.CandidateMemory, 0)
			currentChars = 0
		}
		current = append(current, candidate)
		currentChars += candidateChars
	}
	if len(current) > 0 {
		batches = append(batches, current)
	}
	return batches
}

func targetCount(before int, ratio float64) int {
	target := int(float64(before) * ratio)
	if target < 1 {
		return 1
	}
	if target > before {
		return before
	}
	return target
}

func batchTarget(totalTarget int, batchSize int, totalSize int) int {
	if totalSize <= 0 || batchSize <= 0 {
		return 1
	}
	target := (totalTarget*batchSize + totalSize - 1) / totalSize
	if target < 1 {
		return 1
	}
	if target > batchSize {
		return batchSize
	}
	return target
}

func facts(input []string, limit int) []string {
	seen := make(map[string]struct{}, len(input))
	out := make([]string, 0, len(input))
	for _, fact := range input {
		fact = strings.TrimSpace(fact)
		if fact == "" {
			continue
		}
		if _, ok := seen[fact]; ok {
			continue
		}
		seen[fact] = struct{}{}
		out = append(out, fact)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func archiveItems(items []storefs.MemoryItem, compactedAt time.Time, supersededBy []string) []storefs.MemoryItem {
	archived := make([]storefs.MemoryItem, 0, len(items))
	for _, item := range items {
		item = canonical(item)
		if item.ID == "" || item.Memory == "" {
			continue
		}
		item.Metadata = archiveMetadata(item.Metadata, compactedAt, supersededBy)
		archived = append(archived, item)
	}
	return archived
}

func archiveMetadata(metadata map[string]any, compactedAt time.Time, supersededBy []string) map[string]any {
	out := make(map[string]any, len(metadata)+2)
	for key, value := range metadata {
		out[key] = value
	}
	out["compacted_at"] = compactedAt.UTC().Format(time.RFC3339)
	out["superseded_by"] = append([]string(nil), supersededBy...)
	return out
}

func canonical(item storefs.MemoryItem) storefs.MemoryItem {
	item.ID = strings.TrimSpace(item.ID)
	item.Memory = strings.TrimSpace(item.Memory)
	return item
}

func memoryID(botID string, now time.Time) string {
	return strings.TrimSpace(botID) + ":mem_" + strconv.FormatInt(now.UnixNano(), 10)
}

func memoryHash(text string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return hex.EncodeToString(sum[:])
}
