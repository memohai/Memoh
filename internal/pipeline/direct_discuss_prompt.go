package pipeline

import (
	"context"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/contextfrag"
)

type DirectDiscussPromptSource struct {
	ID          string
	Message     sdk.Message
	Required    bool
	Compactable bool
	SummaryFrag *contextfrag.ContextFrag
	ImageRefs   []ImageAttachmentRef
}

type DirectDiscussPromptInput struct {
	Sources         []DirectDiscussPromptSource
	CurrentSourceID string
	ActorUserID     string
}

type DirectDiscussPromptReceipt interface {
	Finish(context.Context) error
}

type PreparedDirectDiscussPrompt struct {
	RunConfig agentpkg.RunConfig
	Receipt   DirectDiscussPromptReceipt
}

type DirectDiscussPromptPreparer interface {
	PrepareDirectDiscussPrompt(context.Context, DirectDiscussPromptInput) (PreparedDirectDiscussPrompt, error)
}
