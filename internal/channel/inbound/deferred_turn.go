package inbound

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/conversation"
	pipelinepkg "github.com/memohai/memoh/internal/pipeline"
)

type deferredTurn struct {
	id                    string
	ctx                   context.Context
	cfg                   channel.ChannelConfig
	msg                   channel.InboundMessage
	sender                channel.StreamReplySender
	identity              InboundIdentity
	resolved              route.ResolveConversationResult
	resolvedAttachments   []channel.Attachment
	attachments           []conversation.ChatAttachment
	replyAttachments      []conversation.ChatAttachment
	text                  string
	modelText             string
	userMessageKind       string
	userVisibleText       string
	requestedSkills       []conversation.RequestedSkillContext
	skillActivation       *conversation.SkillActivation
	hasPendingSkill       bool
	sessionID             string
	sessionRuntimeOwner   string
	acpRuntimeSession     SessionResult
	activeChatID          string
	inboundMode           InboundMode
	eventID               string
	receipt               conversation.UserMessageReceipt
	routeOwnerTransferred bool
	transferredInjectCh   <-chan conversation.InjectMessage
	activation            *deferredTurnActivation
}

type deferredTurnActivation struct {
	once          sync.Once
	latestContext pipelinepkg.RenderedContext
}

func prepareDeferredTurn(input deferredTurn) (*deferredTurn, error) {
	turn := input
	turn.ctx = context.WithoutCancel(input.ctx)
	var err error
	turn.cfg, err = cloneDeferredValue("channel config", input.cfg)
	if err != nil {
		return nil, err
	}
	turn.msg, err = cloneDeferredValue("inbound message", input.msg)
	if err != nil {
		return nil, err
	}
	turn.resolvedAttachments = append([]channel.Attachment(nil), turn.msg.Message.Attachments...)
	turn.attachments, err = snapshotInboundAttachments(input.attachments)
	if err != nil {
		return nil, err
	}
	turn.replyAttachments, err = snapshotInboundAttachments(input.replyAttachments)
	if err != nil {
		return nil, err
	}
	turn.requestedSkills = append([]conversation.RequestedSkillContext(nil), input.requestedSkills...)
	if input.skillActivation != nil {
		activation := *input.skillActivation
		activation.Skills = append([]conversation.SkillActivationSkill(nil), input.skillActivation.Skills...)
		turn.skillActivation = &activation
	}
	turn.receipt.Metadata, err = cloneDeferredValue("source receipt metadata", input.receipt.Metadata)
	if err != nil {
		return nil, err
	}
	turn.receipt.Attachments, err = snapshotInboundAttachments(input.receipt.Attachments)
	if err != nil {
		return nil, err
	}
	turn.activation = &deferredTurnActivation{}
	return &turn, nil
}

func cloneDeferredValue[T any](name string, value T) (T, error) {
	var cloned T
	raw, err := json.Marshal(value)
	if err != nil {
		return cloned, fmt.Errorf("snapshot deferred %s: %w", name, err)
	}
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return cloned, fmt.Errorf("snapshot deferred %s: %w", name, err)
	}
	return cloned, nil
}

func (t *deferredTurn) ActivateOnce(ctx context.Context, p *ChannelInboundProcessor) pipelinepkg.RenderedContext {
	if t == nil || p == nil || t.activation == nil {
		return nil
	}
	t.activation.once.Do(func() {
		if p.pipeline == nil || strings.TrimSpace(t.sessionID) == "" || t.hasPendingSkill {
			return
		}
		if _, loaded := p.pipeline.GetIC(t.sessionID); !loaded {
			p.replayPipelineSession(ctx, t.sessionID)
		}
		event := pipelinepkg.AdaptInbound(t.msg, t.sessionID, t.identity.ChannelIdentityID, t.identity.DisplayName)
		if p.eventStore != nil {
			eventID, err := p.eventStore.PersistEvent(ctx, t.identity.BotID, t.sessionID, event)
			if err != nil {
				if p.logger != nil {
					p.logger.Warn("persist pipeline event failed", slog.Any("error", err))
				}
			} else {
				t.eventID = eventID
				boundReceipt, bindErr := bindInboundReceiptEventID(t.receipt, eventID)
				if bindErr != nil {
					if p.logger != nil {
						p.logger.Warn("bind inbound source event failed", slog.Any("error", bindErr))
					}
					t.eventID = ""
				} else {
					t.receipt = boundReceipt
				}
			}
		}
		t.activation.latestContext = p.pipeline.PushEvent(t.sessionID, event)
	})
	return t.activation.latestContext
}
