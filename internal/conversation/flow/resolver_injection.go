package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/google/uuid"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
)

type injectionBridge struct {
	cancel    context.CancelFunc
	done      <-chan struct{}
	closeOnce sync.Once
}

func (b *injectionBridge) Close() {
	if b == nil {
		return
	}
	b.closeOnce.Do(func() {
		b.cancel()
		<-b.done
	})
}

func (r *Resolver) startInjectionBridge(
	ctx context.Context,
	req conversation.ChatRequest,
	runConfig agentpkg.RunConfig,
) (agentpkg.RunConfig, *injectionReceiptRegistry, *injectionBridge) {
	messages := req.InjectionFeed.Messages
	if messages == nil {
		return runConfig, nil, nil
	}

	agentMessages := make(chan agentpkg.InjectMessage, cap(messages))
	registry := newInjectionReceiptRegistry()
	bridgeCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	bridge := &injectionBridge{cancel: cancel, done: done}
	go func() {
		defer close(done)
		defer close(agentMessages)
		for {
			var message conversation.InjectMessage
			var open bool
			select {
			case <-bridgeCtx.Done():
				return
			case message, open = <-messages:
				if !open {
					return
				}
			}

			receiptID, receipt, err := registry.admit(message)
			if err != nil {
				if r.logger != nil {
					r.logger.Warn("reject injection receipt",
						slog.String("receipt_id", strings.TrimSpace(message.Receipt.ID)),
						slog.Any("error", err))
				}
				continue
			}
			agentMessage := agentpkg.InjectMessage{
				ReceiptID:       receiptID,
				Text:            receipt.DisplayText,
				HeaderifiedText: message.HeaderifiedText,
			}
			if runConfig.SupportsImageInput && len(receipt.Attachments) > 0 {
				agentMessage.ImageParts = r.inlineInjectAttachments(bridgeCtx, req.BotID, receipt.Attachments)
			}
			select {
			case <-bridgeCtx.Done():
				return
			case agentMessages <- agentMessage:
			}
		}
	}()

	runConfig.InjectCh = agentMessages
	runConfig.InjectedRecorder = func(receipt agentpkg.InjectedReceipt) {
		if registry.record(receipt) {
			return
		}
		if r.logger != nil {
			r.logger.Warn("ignore unknown injection receipt",
				slog.String("receipt_id", string(receipt.ID)))
		}
	}
	return runConfig, registry, bridge
}

func (rc resolvedContext) closeInjectionBridge() {
	rc.injectionBridge.Close()
}

func withoutInjectionCapabilities(req conversation.ChatRequest) conversation.ChatRequest {
	req.InjectionFeed = conversation.InjectionFeed{}
	return req
}

func withoutInjectionRuntime(rc resolvedContext) resolvedContext {
	rc.injectionReceipts = nil
	rc.injectionBridge = nil
	rc.runConfig.InjectCh = nil
	rc.runConfig.InjectedRecorder = nil
	return rc
}

type injectionReceiptRegistry struct {
	mu      sync.Mutex
	pending map[agentpkg.InjectionReceiptID]conversation.UserMessageReceipt
	seen    map[agentpkg.InjectionReceiptID]struct{}
	records []conversation.InjectedMessageRecord
}

var errDuplicateInjectionReceipt = errors.New("duplicate injection receipt")

func newInjectionReceiptRegistry() *injectionReceiptRegistry {
	return &injectionReceiptRegistry{
		pending: make(map[agentpkg.InjectionReceiptID]conversation.UserMessageReceipt),
		seen:    make(map[agentpkg.InjectionReceiptID]struct{}),
		records: make([]conversation.InjectedMessageRecord, 0),
	}
}

func (r *injectionReceiptRegistry) admit(msg conversation.InjectMessage) (agentpkg.InjectionReceiptID, conversation.UserMessageReceipt, error) {
	receipt, err := snapshotInjectionReceipt(msg)
	if err != nil {
		return "", conversation.UserMessageReceipt{}, err
	}
	receiptID := agentpkg.InjectionReceiptID(strings.TrimSpace(receipt.ID))
	if receiptID == "" {
		receiptID = agentpkg.InjectionReceiptID(uuid.NewString())
	}
	receipt.ID = string(receiptID)

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.seen[receiptID]; exists {
		return "", conversation.UserMessageReceipt{}, errDuplicateInjectionReceipt
	}
	r.seen[receiptID] = struct{}{}
	r.pending[receiptID] = receipt
	return receiptID, receipt, nil
}

func (r *injectionReceiptRegistry) record(observed agentpkg.InjectedReceipt) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	receipt, ok := r.pending[observed.ID]
	if !ok {
		return false
	}
	delete(r.pending, observed.ID)
	r.records = append(r.records, conversation.InjectedMessageRecord{
		ModelText:   observed.ModelText,
		Receipt:     receipt,
		InsertAfter: observed.InsertAfter,
	})
	return true
}

func (r *injectionReceiptRegistry) recordsSnapshot() []conversation.InjectedMessageRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]conversation.InjectedMessageRecord(nil), r.records...)
}

func snapshotInjectionReceipt(msg conversation.InjectMessage) (conversation.UserMessageReceipt, error) {
	receipt := msg.Receipt
	receipt.DisplayText = msg.Text
	metadata, err := cloneInjectionMetadata(receipt.Metadata)
	if err != nil {
		return conversation.UserMessageReceipt{}, fmt.Errorf("clone injection metadata: %w", err)
	}
	receipt.Metadata = metadata
	attachments, err := cloneInjectionAttachments(msg.Attachments)
	if err != nil {
		return conversation.UserMessageReceipt{}, err
	}
	receipt.Attachments = attachments
	return receipt, nil
}

func cloneInjectionAttachments(attachments []conversation.ChatAttachment) ([]conversation.ChatAttachment, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	cloned := make([]conversation.ChatAttachment, len(attachments))
	for i, attachment := range attachments {
		cloned[i] = attachment
		metadata, err := cloneInjectionMetadata(attachment.Metadata)
		if err != nil {
			return nil, fmt.Errorf("clone injection attachment %d metadata: %w", i, err)
		}
		cloned[i].Metadata = metadata
	}
	return cloned, nil
}

func cloneInjectionMetadata(metadata map[string]any) (map[string]any, error) {
	if len(metadata) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}
