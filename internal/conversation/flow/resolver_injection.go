package flow

import (
	"context"
	"errors"
	"strings"
	"sync"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
)

var errInjectedMessageNotConsumed = errors.New("injected message was not consumed before the stream ended")

func (r *Resolver) prepareInjectedMessageRun(
	ctx context.Context,
	botID string,
	injectCh <-chan conversation.InjectMessage,
	supportsImageInput bool,
) (<-chan agentpkg.InjectMessage, *[]conversation.InjectedMessageRecord, func(string, int)) {
	agentInjectCh := make(chan agentpkg.InjectMessage)
	pendingSources := make(chan conversation.InjectMessage)
	go func() {
		defer close(agentInjectCh)
		defer close(pendingSources)
		defer rejectPendingInjectedMessages(injectCh, errInjectedMessageNotConsumed)

		seenEventIDs := make(map[string]struct{})
		for {
			var msg conversation.InjectMessage
			var ok bool
			select {
			case <-ctx.Done():
				return
			case msg, ok = <-injectCh:
				if !ok {
					return
				}
			}
			agentMsg := agentpkg.InjectMessage{
				Text:            msg.Text,
				HeaderifiedText: msg.HeaderifiedText,
			}
			if supportsImageInput && len(msg.Attachments) > 0 {
				agentMsg.ImageParts = r.inlineInjectAttachments(ctx, botID, msg.Attachments)
			}
			if !agentInjectMessageHasContent(agentMsg, supportsImageInput) {
				notifyInjectedMessagePersistence(msg, errInjectedMessageNotConsumed)
				continue
			}
			eventID := strings.TrimSpace(msg.Source.EventID)
			if eventID != "" {
				if _, exists := seenEventIDs[eventID]; exists {
					notifyInjectedMessagePersistence(msg, errInjectedMessageNotConsumed)
					continue
				}
				seenEventIDs[eventID] = struct{}{}
			}
			select {
			case agentInjectCh <- agentMsg:
			case <-ctx.Done():
				notifyInjectedMessagePersistence(msg, errInjectedMessageNotConsumed)
				return
			}
			select {
			case pendingSources <- msg:
			case <-ctx.Done():
				notifyInjectedMessagePersistence(msg, errInjectedMessageNotConsumed)
				return
			}
		}
	}()

	records := make([]conversation.InjectedMessageRecord, 0)
	var recordMu sync.Mutex
	recorder := func(headerifiedText string, insertAfter int) {
		injected, ok := <-pendingSources
		if !ok {
			if r.logger != nil {
				r.logger.Error("missing source for injected agent message")
			}
			return
		}
		recordMu.Lock()
		defer recordMu.Unlock()
		injected.HeaderifiedText = headerifiedText
		records = append(records, conversation.InjectedMessageRecord{
			Message:     injected,
			AfterOutput: insertAfter,
			Sequence:    len(records),
		})
	}
	return agentInjectCh, &records, recorder
}

func rejectPendingInjectedMessages(ch <-chan conversation.InjectMessage, err error) {
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			notifyInjectedMessagePersistence(msg, err)
		default:
			return
		}
	}
}

func notifyInjectedMessagePersistence(msg conversation.InjectMessage, err error) {
	if msg.OnPersisted != nil {
		msg.OnPersisted(err)
	}
}

func notifyInjectedPersistence(records *[]conversation.InjectedMessageRecord, err error) {
	if records == nil {
		return
	}
	for _, record := range *records {
		notifyInjectedMessagePersistence(record.Message, err)
	}
}

func buildInjectedRouteMetadata(source conversation.InjectedMessageSource) map[string]any {
	meta := map[string]any{}
	if value := strings.TrimSpace(source.RouteID); value != "" {
		meta["route_id"] = value
	}
	if value := strings.TrimSpace(source.Platform); value != "" {
		meta["platform"] = value
	}
	if source.EventCursor > 0 {
		meta["source_event_cursor"] = source.EventCursor
	}
	if source.ReceivedAtMs > 0 {
		meta["source_received_at_ms"] = source.ReceivedAtMs
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}

func buildInjectedInteractionMetadata(injected conversation.InjectMessage) map[string]any {
	source := injected.Source
	meta := map[string]any{}
	reply := map[string]any{}
	if value := strings.TrimSpace(source.SourceReplyToMessageID); value != "" {
		reply["message_id"] = value
	}
	if value := strings.TrimSpace(source.ReplySender); value != "" {
		reply["sender"] = value
	}
	if value := strings.TrimSpace(source.ReplyPreview); value != "" {
		reply["preview"] = value
	}
	if attachments := chatAttachmentMetadata(source.ReplyAttachments); len(attachments) > 0 {
		reply["attachments"] = attachments
	}
	if len(reply) > 0 {
		meta["reply"] = reply
	}

	forward := map[string]any{}
	if value := strings.TrimSpace(source.ForwardMessageID); value != "" {
		forward["message_id"] = value
	}
	if value := strings.TrimSpace(source.ForwardFromUserID); value != "" {
		forward["from_user_id"] = value
	}
	if value := strings.TrimSpace(source.ForwardFromConversationID); value != "" {
		forward["from_conversation_id"] = value
	}
	if value := strings.TrimSpace(source.ForwardSender); value != "" {
		forward["sender"] = value
	}
	if source.ForwardDate > 0 {
		forward["date"] = source.ForwardDate
	}
	if len(forward) > 0 {
		meta["forward"] = forward
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}
