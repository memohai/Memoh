package flow

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/models"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

func TestTrimDiscussContextKeepsSummariesAndNotice(t *testing.T) {
	t.Parallel()

	r := &Resolver{logger: slog.New(slog.DiscardHandler)}
	summary := pipelinepkg.ContextMessage{Role: "user", Content: "<summary>\ncondensed history\n</summary>", CompactionArtifactID: "artifact-a"}
	old := pipelinepkg.ContextMessage{Role: "user", Content: strings.Repeat("old context ", 100)}
	latest := pipelinepkg.ContextMessage{Role: "user", Content: "latest trigger"}
	budget := estimateMessageTokens(contextMessageForMetering(latest))

	messages, estimated := r.TrimDiscussContext([]pipelinepkg.ContextMessage{summary, old, latest}, budget, 0)

	if len(messages) != 3 {
		t.Fatalf("messages = %d, want notice+summary+latest: %#v", len(messages), messages)
	}
	if messages[0].Role != "system" || !strings.Contains(messages[0].Content, "trimmed") {
		t.Fatalf("missing truncation notice: %#v", messages[0])
	}
	if messages[1].CompactionArtifactID != "artifact-a" {
		t.Fatalf("summary not retained: %#v", messages)
	}
	if messages[2].Content != "latest trigger" {
		t.Fatalf("latest trigger not retained: %#v", messages)
	}
	wantEstimate := 0
	for _, message := range messages {
		wantEstimate += estimateMessageTokens(contextMessageForMetering(message))
	}
	if estimated != wantEstimate {
		t.Fatalf("estimated = %d, want %d", estimated, wantEstimate)
	}
}

func TestTrimDiscussContextWithoutBudgetKeepsEverything(t *testing.T) {
	t.Parallel()

	r := &Resolver{logger: slog.New(slog.DiscardHandler)}
	old := pipelinepkg.ContextMessage{Role: "user", Content: "old context"}
	latest := pipelinepkg.ContextMessage{Role: "user", Content: "latest trigger"}

	messages, estimated := r.TrimDiscussContext([]pipelinepkg.ContextMessage{old, latest}, 0, 0)

	want := estimateMessageTokens(contextMessageForMetering(old)) + estimateMessageTokens(contextMessageForMetering(latest))
	if len(messages) != 2 || estimated != want {
		t.Fatalf("untrimmed passthrough broken: %d messages, estimate %d want %d", len(messages), estimated, want)
	}
}

func TestModelContextTokenBudget(t *testing.T) {
	t.Parallel()

	window := 128000
	if got := modelContextTokenBudget(models.GetResponse{Model: models.Model{Config: models.ModelConfig{ContextWindow: &window}}}); got != 128000 {
		t.Fatalf("declared window budget = %d, want 128000", got)
	}
	if got := modelContextTokenBudget(models.GetResponse{}); got != 0 {
		t.Fatalf("undeclared window budget = %d, want 0", got)
	}
}

func TestTrimDiscussContextPinsLatestUserTriggerOverNewerTurnResponse(t *testing.T) {
	t.Parallel()

	r := &Resolver{logger: slog.New(slog.DiscardHandler)}
	trigger := pipelinepkg.ContextMessage{Role: "user", Content: strings.Repeat("triggering question ", 30)}
	reply := pipelinepkg.ContextMessage{Role: "assistant", Content: "previous reply"}
	budget := estimateMessageTokens(contextMessageForMetering(reply)) + 1

	messages, _ := r.TrimDiscussContext([]pipelinepkg.ContextMessage{trigger, reply}, budget, 0)

	foundTrigger := false
	for _, message := range messages {
		if message.Role == "user" && strings.Contains(message.Content, "triggering question") {
			foundTrigger = true
		}
	}
	if !foundTrigger {
		t.Fatalf("the triggering user message was trimmed away while the bot's own reply survived: %#v", messages)
	}
}

func TestTrimDiscussContextPinsOldUserMessageWhoseEventCursorTriggeredRun(t *testing.T) {
	t.Parallel()

	r := &Resolver{logger: slog.New(slog.DiscardHandler)}
	editedTrigger := pipelinepkg.ContextMessage{
		Role:                      "user",
		Content:                   strings.Repeat("edited old question ", 30),
		LatestExternalEventCursor: 300,
	}
	previousReply := pipelinepkg.ContextMessage{Role: "assistant", Content: "previous reply"}
	newerUser := pipelinepkg.ContextMessage{Role: "user", Content: "newer conversation message", LatestExternalEventCursor: 200}
	budget := estimateMessageTokens(contextMessageForMetering(newerUser)) + 1

	messages, _ := r.TrimDiscussContext(
		[]pipelinepkg.ContextMessage{editedTrigger, previousReply, newerUser},
		budget,
		250,
	)

	for _, message := range messages {
		if strings.Contains(message.Content, "edited old question") {
			return
		}
	}
	t.Fatalf("the user message whose edit triggered the run was trimmed away: %#v", messages)
}

func TestTrimDiscussContextPinsExactTriggerBelowProcessedCursor(t *testing.T) {
	t.Parallel()

	r := &Resolver{logger: slog.New(slog.DiscardHandler)}
	unselectedHigherCursor := pipelinepkg.ContextMessage{
		Role:                      "user",
		Content:                   strings.Repeat("unselected higher cursor ", 30),
		LatestExternalEventCursor: 30,
		CurrentTriggerEvaluated:   true,
	}
	delayedTrigger := pipelinepkg.ContextMessage{
		Role:                      "user",
		Content:                   strings.Repeat("delayed exact trigger ", 30),
		LatestExternalEventCursor: 10,
		CurrentTrigger:            true,
		CurrentTriggerEvaluated:   true,
	}
	previousReply := pipelinepkg.ContextMessage{Role: "assistant", Content: "previous reply"}
	budget := estimateMessageTokens(contextMessageForMetering(delayedTrigger)) + 1

	messages, _ := r.TrimDiscussContext(
		[]pipelinepkg.ContextMessage{unselectedHigherCursor, previousReply, delayedTrigger},
		budget,
		20,
	)

	foundTrigger := false
	for _, message := range messages {
		if strings.Contains(message.Content, "delayed exact trigger") {
			foundTrigger = true
		}
		if strings.Contains(message.Content, "unselected higher cursor") {
			t.Fatalf("unselected higher-cursor message was pinned as current: %#v", messages)
		}
	}
	if !foundTrigger {
		t.Fatalf("the exact delayed trigger was trimmed away: %#v", messages)
	}
}

func TestTrimDiscussContextDoesNotPinUnselectedUserAfterExactEvaluation(t *testing.T) {
	t.Parallel()

	r := &Resolver{logger: slog.New(slog.DiscardHandler)}
	unselected := pipelinepkg.ContextMessage{
		Role:                    "user",
		Content:                 strings.Repeat("unselected exact context ", 30),
		CurrentTriggerEvaluated: true,
	}
	reply := pipelinepkg.ContextMessage{Role: "assistant", Content: "reply to keep"}
	budget := estimateMessageTokens(contextMessageForMetering(reply)) + 1

	messages, _ := r.TrimDiscussContext([]pipelinepkg.ContextMessage{unselected, reply}, budget, 20)

	for _, message := range messages {
		if strings.Contains(message.Content, "unselected exact context") {
			t.Fatalf("unselected evaluated user was pinned by the legacy fallback: %#v", messages)
		}
	}
}

func TestTrimDiscussContextNeverEmitsOrphanToolResults(t *testing.T) {
	t.Parallel()

	r := &Resolver{logger: slog.New(slog.DiscardHandler)}
	call := pipelinepkg.ContextMessage{Role: "assistant", RawContent: json.RawMessage(`[{"type":"tool_call","tool_call_id":"call-1","tool_name":"exec","input":{"cmd":"` + strings.Repeat("x", 400) + `"}}]`)}
	result := pipelinepkg.ContextMessage{Role: "tool", RawContent: json.RawMessage(`[{"type":"tool_result","tool_call_id":"call-1","tool_name":"exec","result":"ok"}]`)}
	user := pipelinepkg.ContextMessage{Role: "user", Content: "please run it"}
	budget := estimateMessageTokens(contextMessageForMetering(result)) + 1

	messages, estimated := r.TrimDiscussContext([]pipelinepkg.ContextMessage{user, call, result}, budget, 0)

	haveCall := false
	for _, message := range messages {
		if message.Role == "assistant" {
			haveCall = true
		}
	}
	wantEstimate := 0
	for _, message := range messages {
		if message.Role == "tool" && !haveCall {
			t.Fatalf("orphan tool result emitted without its call: %#v", messages)
		}
		wantEstimate += estimateMessageTokens(contextMessageForMetering(message))
	}
	if estimated != wantEstimate {
		t.Fatalf("estimate = %d counts messages that were not emitted (want %d)", estimated, wantEstimate)
	}
	foundTrigger := false
	for _, message := range messages {
		if message.Role == "user" && strings.Contains(message.Content, "please run it") {
			foundTrigger = true
		}
	}
	if !foundTrigger {
		t.Fatalf("latest user trigger lost: %#v", messages)
	}
}
