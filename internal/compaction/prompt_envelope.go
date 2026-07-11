package compaction

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/messageconv"
	"github.com/memohai/memoh/internal/models"
)

const (
	maxCompactionSummaryTokens = 4096
	compactionHeadroomPercent  = 5
)

var (
	errCompactionBudgetUnknown          = errors.New("compaction: model context budget is unknown")
	errCompactionEnvelopeTooSmall       = errors.New("compaction: model context budget cannot fit a complete candidate group")
	errCompactionOutputLimitUnsupported = errors.New("compaction: model provider does not enforce the summary output limit")
)

type compactionEnvelopeRequest struct {
	Model              *sdk.Model
	PromptCacheTTL     string
	ContextTokenBudget int
	Candidates         []CompactionCandidate
	Artifacts          []Artifact
}

type compactionEnvelope struct {
	System               string
	Messages             []sdk.Message
	Candidates           []CompactionCandidate
	MessageIDs           []pgtype.UUID
	InputTokenUpperBound int
	MaxOutputTokens      int
	HeadroomTokens       int
	RawReplayTokens      int
}

type preparedCompactionPrompt struct {
	system     string
	messages   []sdk.Message
	upperBound int
}

func materializeCompactionEnvelope(request compactionEnvelopeRequest) (compactionEnvelope, error) {
	if request.ContextTokenBudget <= 0 {
		return compactionEnvelope{}, errCompactionBudgetUnknown
	}
	if models.ResolveClientType(request.Model) == string(models.ClientTypeOpenAICodex) {
		return compactionEnvelope{}, errCompactionOutputLimitUnsupported
	}
	maxOutputTokens := min(maxCompactionSummaryTokens, max(1, request.ContextTokenBudget/10))
	headroomTokens := max(1, request.ContextTokenBudget*compactionHeadroomPercent/100)
	inputTokenBudget := request.ContextTokenBudget - maxOutputTokens - headroomTokens
	if inputTokenBudget <= 0 {
		return compactionEnvelope{}, fmt.Errorf(
			"%w: context=%d output=%d headroom=%d",
			errCompactionEnvelopeTooSmall,
			request.ContextTokenBudget,
			maxOutputTokens,
			headroomTokens,
		)
	}

	entries, messageIDs := buildEntriesAndIDs(request.Candidates)
	if len(entries) == 0 || len(messageIDs) == 0 {
		return compactionEnvelope{}, nil
	}
	candidates, ok := candidatesForIDs(request.Candidates, messageIDs)
	if !ok {
		return compactionEnvelope{}, errors.New("compaction: rendered candidate identity mismatch")
	}

	groups := toolExchangeGroups(candidates)
	for len(groups) > 0 {
		prompt, err := prepareCompactionPrompt(request.Model, request.PromptCacheTTL, nil, entries)
		if err != nil {
			return compactionEnvelope{}, err
		}
		if prompt.upperBound <= inputTokenBudget {
			envelope := compactionEnvelope{
				System:               prompt.system,
				Messages:             prompt.messages,
				Candidates:           append([]CompactionCandidate(nil), candidates...),
				MessageIDs:           append([]pgtype.UUID(nil), messageIDs...),
				InputTokenUpperBound: prompt.upperBound,
				MaxOutputTokens:      maxOutputTokens,
				HeadroomTokens:       headroomTokens,
				RawReplayTokens:      estimateCandidateReplayTokens(candidates),
			}
			return addPriorArtifacts(envelope, request, entries, inputTokenBudget)
		}
		if len(groups) == 1 {
			return compactionEnvelope{}, fmt.Errorf(
				"%w: required=%d available=%d",
				errCompactionEnvelopeTooSmall,
				prompt.upperBound,
				inputTokenBudget,
			)
		}
		dropCount := len(groups[0])
		candidates = candidates[dropCount:]
		entries = entries[dropCount:]
		messageIDs = messageIDs[dropCount:]
		groups = toolExchangeGroups(candidates)
	}
	return compactionEnvelope{}, nil
}

func candidatesForIDs(items []CompactionCandidate, ids []pgtype.UUID) ([]CompactionCandidate, bool) {
	byID := make(map[pgtype.UUID]CompactionCandidate, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	selected := make([]CompactionCandidate, len(ids))
	for index, id := range ids {
		item, ok := byID[id]
		if !ok {
			return nil, false
		}
		selected[index] = item
	}
	return selected, true
}

func prepareCompactionPrompt(model *sdk.Model, promptCacheTTL string, priorSummaries []string, entries []messageEntry) (preparedCompactionPrompt, error) {
	userPrompt := buildUserPrompt(priorSummaries, entries)
	system, messages, _ := models.ApplyPromptCache(
		model,
		promptCacheTTL,
		systemPrompt,
		[]sdk.Message{sdk.UserMessage(userPrompt)},
		nil,
	)
	// A model token cannot consume less than one UTF-8 byte. Counting the
	// serialized prompt bytes therefore stays safe without a provider tokenizer;
	// the separate headroom covers provider-specific framing outside this shape.
	encoded, err := json.Marshal(struct {
		System   string        `json:"system,omitempty"`
		Messages []sdk.Message `json:"messages"`
	}{System: system, Messages: messages})
	if err != nil {
		return preparedCompactionPrompt{}, fmt.Errorf("marshal compaction prompt envelope: %w", err)
	}
	return preparedCompactionPrompt{system: system, messages: messages, upperBound: len(encoded)}, nil
}

func addPriorArtifacts(
	envelope compactionEnvelope,
	request compactionEnvelopeRequest,
	entries []messageEntry,
	inputTokenBudget int,
) (compactionEnvelope, error) {
	candidateStartMs := firstCandidateTimestamp(envelope.Candidates)
	if candidateStartMs <= 0 {
		return envelope, nil
	}
	eligible := make([]Artifact, 0, len(request.Artifacts))
	for _, artifact := range request.Artifacts {
		if strings.TrimSpace(artifact.Summary) == "" || artifact.AnchorEndMs <= 0 || artifact.AnchorEndMs >= candidateStartMs {
			continue
		}
		eligible = append(eligible, artifact)
	}
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].AnchorEndMs != eligible[j].AnchorEndMs {
			return eligible[i].AnchorEndMs < eligible[j].AnchorEndMs
		}
		if eligible[i].AnchorStartMs != eligible[j].AnchorStartMs {
			return eligible[i].AnchorStartMs < eligible[j].AnchorStartMs
		}
		return eligible[i].ID < eligible[j].ID
	})

	selected := make([]Artifact, 0, len(eligible))
	for index := len(eligible) - 1; index >= 0; index-- {
		proposed := append([]Artifact{eligible[index]}, selected...)
		priorSummaries := make([]string, len(proposed))
		for artifactIndex, artifact := range proposed {
			priorSummaries[artifactIndex] = artifact.Summary
		}
		prompt, err := prepareCompactionPrompt(request.Model, request.PromptCacheTTL, priorSummaries, entries)
		if err != nil {
			return compactionEnvelope{}, err
		}
		if prompt.upperBound > inputTokenBudget {
			continue
		}
		selected = proposed
		envelope.System = prompt.system
		envelope.Messages = prompt.messages
		envelope.InputTokenUpperBound = prompt.upperBound
	}
	return envelope, nil
}

func firstCandidateTimestamp(candidates []CompactionCandidate) int64 {
	if len(candidates) == 0 || candidates[0].Record.CreatedAt.IsZero() {
		return 0
	}
	return candidates[0].Record.CreatedAt.UnixMilli()
}

func estimateCandidateReplayTokens(candidates []CompactionCandidate) int {
	tokens := 0
	for _, candidate := range candidates {
		tokens += messageconv.EstimateModelMessageTokens(candidate.Record.ModelMessage)
	}
	return tokens
}

func estimateSummaryReplayTokens(summary string) int {
	return messageconv.EstimateSDKMessageTokens(sdk.UserMessage("<summary>\n" + strings.TrimSpace(summary) + "\n</summary>"))
}
