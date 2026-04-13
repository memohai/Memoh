package pipeline

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
)

func TestExtractNewImageRefs(t *testing.T) {
	rc := RenderedContext{
		{ReceivedAtMs: 100, ImageRefs: []ImageAttachmentRef{{ContentHash: "old-hash", Mime: "image/png"}}},
		{ReceivedAtMs: 200, IsMyself: true, ImageRefs: []ImageAttachmentRef{{ContentHash: "self-hash"}}},
		{ReceivedAtMs: 300, ImageRefs: []ImageAttachmentRef{{ContentHash: "new-hash", Mime: "image/jpeg"}}},
		{ReceivedAtMs: 400, ImageRefs: nil},
	}

	refs := extractNewImageRefs(rc, 150)
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d", len(refs))
	}
	if refs[0].ContentHash != "new-hash" {
		t.Fatalf("expected new-hash, got %q", refs[0].ContentHash)
	}
	if refs[0].Mime != "image/jpeg" {
		t.Fatalf("expected image/jpeg, got %q", refs[0].Mime)
	}
}

func TestExtractNewImageRefs_IncludesMultiple(t *testing.T) {
	rc := RenderedContext{
		{ReceivedAtMs: 100},
		{ReceivedAtMs: 200, ImageRefs: []ImageAttachmentRef{
			{ContentHash: "a"},
			{ContentHash: "b"},
		}},
		{ReceivedAtMs: 300, ImageRefs: []ImageAttachmentRef{{ContentHash: "c"}}},
	}
	refs := extractNewImageRefs(rc, 50)
	if len(refs) != 3 {
		t.Fatalf("expected 3 refs, got %d", len(refs))
	}
}

func TestInjectImagePartsIntoLastUserMessage(t *testing.T) {
	msgs := []sdk.Message{
		sdk.UserMessage("hello"),
		sdk.AssistantMessage("hi"),
		sdk.UserMessage("look at this"),
	}
	parts := []sdk.ImagePart{
		{Image: "data:image/png;base64,abc", MediaType: "image/png"},
	}

	injectImagePartsIntoLastUserMessage(msgs, parts)

	lastUser := msgs[2]
	if len(lastUser.Content) != 2 {
		t.Fatalf("expected 2 content parts, got %d", len(lastUser.Content))
	}
	imgPart, ok := lastUser.Content[1].(sdk.ImagePart)
	if !ok {
		t.Fatalf("expected ImagePart, got %T", lastUser.Content[1])
	}
	if imgPart.Image != "data:image/png;base64,abc" {
		t.Fatalf("unexpected image: %q", imgPart.Image)
	}
}

func TestInjectImagePartsIntoLastUserMessage_Empty(t *testing.T) {
	msgs := []sdk.Message{sdk.UserMessage("hello")}
	injectImagePartsIntoLastUserMessage(msgs, nil)
	if len(msgs[0].Content) != 1 {
		t.Fatalf("expected no change, got %d parts", len(msgs[0].Content))
	}
}

func TestInjectImagePartsIntoLastUserMessage_SkipsEmptyImage(t *testing.T) {
	msgs := []sdk.Message{sdk.UserMessage("hello")}
	parts := []sdk.ImagePart{{Image: "", MediaType: "image/png"}}
	injectImagePartsIntoLastUserMessage(msgs, parts)
	if len(msgs[0].Content) != 1 {
		t.Fatalf("expected no change, got %d parts", len(msgs[0].Content))
	}
}

func TestHandleReplyWithAgent_InlinesImages(t *testing.T) {
	rc := RenderedContext{
		{
			ReceivedAtMs: 200,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="1">photo</message>`}},
			ImageRefs:    []ImageAttachmentRef{{ContentHash: "img-hash", Mime: "image/jpeg"}},
		},
	}

	fakeAgent := &fakeDiscussStreamer{}

	resolver := &fakeRunConfigResolver{
		resolveResult: ResolveRunConfigResult{
			RunConfig: agentpkg.RunConfig{
				SupportsImageInput: true,
			},
			ModelID: "model-1",
		},
		inlineFn: func(_ context.Context, _ string, refs []ImageAttachmentRef) []sdk.ImagePart {
			if len(refs) != 1 || refs[0].ContentHash != "img-hash" {
				t.Fatalf("unexpected refs: %v", refs)
			}
			return []sdk.ImagePart{{Image: "data:image/jpeg;base64,FAKE", MediaType: "image/jpeg"}}
		},
	}

	driver := NewDiscussDriver(DiscussDriverDeps{
		Pipeline: NewPipeline(RenderParams{}),
		Resolver: resolver,
	})

	sess := &discussSession{
		config: DiscussSessionConfig{
			BotID:     "bot-1",
			SessionID: "sess-1",
		},
		lastProcessedMs: 0,
	}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, fakeAgent)

	if fakeAgent.lastConfig == nil {
		t.Fatal("expected agent to be called")
	}

	msgs := fakeAgent.lastConfig.Messages
	var userMsgs []sdk.Message
	for _, m := range msgs {
		if m.Role == sdk.MessageRoleUser {
			userMsgs = append(userMsgs, m)
		}
	}
	if len(userMsgs) < 2 {
		t.Fatalf("expected at least 2 user messages (rc + late binding), got %d", len(userMsgs))
	}
	rcMsg := userMsgs[0]
	hasImage := false
	for _, part := range rcMsg.Content {
		if imgPart, ok := part.(sdk.ImagePart); ok {
			hasImage = true
			if !strings.HasPrefix(imgPart.Image, "data:image/jpeg;base64,") {
				t.Fatalf("unexpected image data: %q", imgPart.Image)
			}
		}
	}
	if !hasImage {
		t.Fatal("expected image part in RC user message")
	}
}

func TestHandleReplyWithAgent_NoInlineWhenNoVision(t *testing.T) {
	rc := RenderedContext{
		{
			ReceivedAtMs: 200,
			Content:      []RenderedContentPiece{{Type: "text", Text: `<message id="1">photo</message>`}},
			ImageRefs:    []ImageAttachmentRef{{ContentHash: "img-hash", Mime: "image/jpeg"}},
		},
	}

	fakeAgent := &fakeDiscussStreamer{}

	resolver := &fakeRunConfigResolver{
		resolveResult: ResolveRunConfigResult{
			RunConfig: agentpkg.RunConfig{
				SupportsImageInput: false,
			},
			ModelID: "model-1",
		},
		inlineFn: func(_ context.Context, _ string, _ []ImageAttachmentRef) []sdk.ImagePart {
			t.Fatal("should not be called when model doesn't support vision")
			return nil
		},
	}

	driver := NewDiscussDriver(DiscussDriverDeps{
		Pipeline: NewPipeline(RenderParams{}),
		Resolver: resolver,
	})

	sess := &discussSession{
		config: DiscussSessionConfig{
			BotID:     "bot-1",
			SessionID: "sess-1",
		},
		lastProcessedMs: 0,
	}

	driver.handleReplyWithAgent(context.Background(), sess, rc, driver.logger, fakeAgent)

	if fakeAgent.lastConfig == nil {
		t.Fatal("expected agent to be called")
	}
	for _, m := range fakeAgent.lastConfig.Messages {
		for _, part := range m.Content {
			if _, ok := part.(sdk.ImagePart); ok {
				t.Fatal("should not have image parts when vision is not supported")
			}
		}
	}
}

// --- Test helpers ---

type fakeDiscussStreamer struct {
	lastConfig *agentpkg.RunConfig
}

func (f *fakeDiscussStreamer) Stream(_ context.Context, cfg agentpkg.RunConfig) <-chan agentpkg.StreamEvent {
	f.lastConfig = &cfg
	ch := make(chan agentpkg.StreamEvent, 1)
	ch <- agentpkg.StreamEvent{Type: agentpkg.EventAgentEnd}
	close(ch)
	return ch
}

type fakeRunConfigResolver struct {
	resolveResult ResolveRunConfigResult
	inlineFn      func(ctx context.Context, botID string, refs []ImageAttachmentRef) []sdk.ImagePart
}

func (f *fakeRunConfigResolver) ResolveRunConfig(_ context.Context, _, _, _, _, _, _, _ string) (ResolveRunConfigResult, error) {
	return f.resolveResult, nil
}

func (f *fakeRunConfigResolver) InlineImageAttachments(ctx context.Context, botID string, refs []ImageAttachmentRef) []sdk.ImagePart {
	if f.inlineFn != nil {
		return f.inlineFn(ctx, botID, refs)
	}
	return nil
}

func (*fakeRunConfigResolver) StoreRound(_ context.Context, _, _, _, _ string, _ []sdk.Message, _ string) error {
	return nil
}
