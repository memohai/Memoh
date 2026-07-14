package agent

import (
	"sync"

	sdk "github.com/memohai/twilight-ai/sdk"
)

type preparedPrompt struct {
	System   string
	Messages []sdk.Message
}

type preparedPromptTracker struct {
	mu     sync.Mutex
	prompt preparedPrompt
	set    bool
}

func (t *preparedPromptTracker) Store(params *sdk.GenerateParams) {
	if t == nil || params == nil {
		return
	}
	t.mu.Lock()
	t.prompt = preparedPrompt{
		System:   params.System,
		Messages: clonePreparedMessages(params.Messages),
	}
	t.set = true
	t.mu.Unlock()
}

func (t *preparedPromptTracker) Load() (preparedPrompt, bool) {
	if t == nil {
		return preparedPrompt{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.set {
		return preparedPrompt{}, false
	}
	return preparedPrompt{
		System:   t.prompt.System,
		Messages: clonePreparedMessages(t.prompt.Messages),
	}, true
}

func clonePreparedMessages(messages []sdk.Message) []sdk.Message {
	cloned := make([]sdk.Message, len(messages))
	for i, message := range messages {
		cloned[i] = message
		cloned[i].Content = append([]sdk.MessagePart(nil), message.Content...)
	}
	return cloned
}
