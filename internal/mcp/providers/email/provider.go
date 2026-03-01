package email

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/memohai/memoh/internal/email"
	mcpgw "github.com/memohai/memoh/internal/mcp"
)

const (
	toolEmailAccounts = "email_accounts"
	toolEmailSend     = "email_send"
	toolEmailList     = "email_list"
	toolEmailRead     = "email_read"
)

type Executor struct {
	logger  *slog.Logger
	service *email.Service
	manager *email.Manager
}

func NewExecutor(log *slog.Logger, service *email.Service, manager *email.Manager) *Executor {
	return &Executor{
		logger:  log.With(slog.String("provider", "email_tool")),
		service: service,
		manager: manager,
	}
}

func (e *Executor) ListTools(_ context.Context, _ mcpgw.ToolSessionContext) ([]mcpgw.ToolDescriptor, error) {
	return []mcpgw.ToolDescriptor{
		{
			Name:        toolEmailAccounts,
			Description: "List the email accounts (provider bindings) configured for this bot, including provider IDs, email addresses, and permissions.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        toolEmailSend,
			Description: "Send an email via the bot's configured email provider. Requires write permission.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"to":          map[string]any{"type": "string", "description": "Recipient email address(es), comma-separated"},
					"subject":     map[string]any{"type": "string", "description": "Email subject"},
					"body":        map[string]any{"type": "string", "description": "Email body content"},
					"html":        map[string]any{"type": "boolean", "description": "Whether body is HTML (default false)"},
					"provider_id": map[string]any{"type": "string", "description": "Email provider ID to send from (optional, uses default if omitted)"},
				},
				"required": []string{"to", "subject", "body"},
			},
		},
		{
			Name:        toolEmailList,
			Description: "List emails from the mailbox (newest first). Supports pagination. Requires read permission.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"page":        map[string]any{"type": "integer", "description": "Page number, 0-based (default 0 = newest)"},
					"page_size":   map[string]any{"type": "integer", "description": "Emails per page (default 20)"},
					"provider_id": map[string]any{"type": "string", "description": "Email provider ID (optional, uses first readable binding)"},
				},
			},
		},
		{
			Name:        toolEmailRead,
			Description: "Read the full content of an email by its UID. Requires read permission.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"uid":         map[string]any{"type": "integer", "description": "The email UID from email_list results"},
					"provider_id": map[string]any{"type": "string", "description": "Email provider ID (optional, uses first readable binding)"},
				},
				"required": []string{"uid"},
			},
		},
	}, nil
}

func (e *Executor) CallTool(ctx context.Context, session mcpgw.ToolSessionContext, toolName string, arguments map[string]any) (map[string]any, error) {
	botID := strings.TrimSpace(session.BotID)
	if botID == "" {
		return mcpgw.BuildToolErrorResult("bot_id is required"), nil
	}

	bindings, err := e.service.ListBindings(ctx, botID)
	if err != nil || len(bindings) == 0 {
		return mcpgw.BuildToolErrorResult("no email binding configured for this bot"), nil
	}

	resolveReadBinding := func() *email.BindingResponse {
		providerID := mcpgw.StringArg(arguments, "provider_id")
		for i := range bindings {
			if !bindings[i].CanRead {
				continue
			}
			if providerID == "" || bindings[i].EmailProviderID == providerID {
				return &bindings[i]
			}
		}
		return nil
	}

	resolveWriteBinding := func() *email.BindingResponse {
		providerID := mcpgw.StringArg(arguments, "provider_id")
		for i := range bindings {
			if !bindings[i].CanWrite {
				continue
			}
			if providerID == "" || bindings[i].EmailProviderID == providerID {
				return &bindings[i]
			}
		}
		return nil
	}

	switch toolName {
	case toolEmailAccounts:
		return e.callAccounts(ctx, bindings)
	case toolEmailSend:
		binding := resolveWriteBinding()
		if binding == nil {
			return mcpgw.BuildToolErrorResult("email write permission denied or provider not found"), nil
		}
		return e.callSend(ctx, botID, binding.EmailProviderID, arguments)
	case toolEmailList:
		binding := resolveReadBinding()
		if binding == nil {
			return mcpgw.BuildToolErrorResult("email read permission denied or provider not found"), nil
		}
		return e.callList(ctx, binding.EmailProviderID, arguments)
	case toolEmailRead:
		binding := resolveReadBinding()
		if binding == nil {
			return mcpgw.BuildToolErrorResult("email read permission denied or provider not found"), nil
		}
		return e.callRead(ctx, binding.EmailProviderID, arguments)
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

func (e *Executor) callAccounts(_ context.Context, bindings []email.BindingResponse) (map[string]any, error) {
	accounts := make([]map[string]any, 0, len(bindings))
	for _, b := range bindings {
		accounts = append(accounts, map[string]any{
			"provider_id":   b.EmailProviderID,
			"email_address": b.EmailAddress,
			"can_read":      b.CanRead,
			"can_write":     b.CanWrite,
			"can_delete":    b.CanDelete,
		})
	}
	return mcpgw.BuildToolSuccessResult(map[string]any{
		"accounts": accounts,
	}), nil
}

func (e *Executor) callSend(ctx context.Context, botID string, providerID string, args map[string]any) (map[string]any, error) {
	toRaw := mcpgw.StringArg(args, "to")
	subject := mcpgw.StringArg(args, "subject")
	body := mcpgw.StringArg(args, "body")
	isHTML, _, _ := mcpgw.BoolArg(args, "html")

	if toRaw == "" || subject == "" || body == "" {
		return mcpgw.BuildToolErrorResult("to, subject, and body are required"), nil
	}

	var toList []string
	for _, addr := range strings.Split(toRaw, ",") {
		addr = strings.TrimSpace(addr)
		if addr != "" {
			toList = append(toList, addr)
		}
	}

	msg := email.OutboundEmail{
		To:      toList,
		Subject: subject,
		Body:    body,
		HTML:    isHTML,
	}

	messageID, err := e.manager.SendEmail(ctx, botID, providerID, msg)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	return mcpgw.BuildToolSuccessResult(map[string]any{
		"message_id": messageID,
		"status":     "sent",
	}), nil
}

func (e *Executor) callList(ctx context.Context, providerID string, args map[string]any) (map[string]any, error) {
	providerName, config, err := e.service.ProviderConfig(ctx, providerID)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	reader, err := e.service.Registry().GetMailboxReader(providerName)
	if err != nil {
		return mcpgw.BuildToolErrorResult("mailbox listing not supported for this provider"), nil
	}

	page, _, _ := mcpgw.IntArg(args, "page")
	pageSize, _, _ := mcpgw.IntArg(args, "page_size")
	if pageSize <= 0 {
		pageSize = 20
	}

	emails, total, err := reader.ListMailbox(ctx, config, page, pageSize)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	summaries := make([]map[string]any, 0, len(emails))
	for _, item := range emails {
		summaries = append(summaries, map[string]any{
			"uid":         item.MessageID,
			"from":        item.From,
			"subject":     item.Subject,
			"received_at": item.ReceivedAt,
		})
	}

	return mcpgw.BuildToolSuccessResult(map[string]any{
		"emails": summaries,
		"total":  total,
		"page":   page,
	}), nil
}

func (e *Executor) callRead(ctx context.Context, providerID string, args map[string]any) (map[string]any, error) {
	uidRaw, ok, _ := mcpgw.IntArg(args, "uid")
	if !ok || uidRaw <= 0 {
		uidStr := mcpgw.StringArg(args, "uid")
		if uidStr != "" {
			parsed, _ := strconv.Atoi(uidStr)
			uidRaw = parsed
		}
	}
	if uidRaw <= 0 {
		return mcpgw.BuildToolErrorResult("uid is required"), nil
	}

	providerName, config, err := e.service.ProviderConfig(ctx, providerID)
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	reader, err := e.service.Registry().GetMailboxReader(providerName)
	if err != nil {
		return mcpgw.BuildToolErrorResult("mailbox reading not supported for this provider"), nil
	}

	item, err := reader.ReadMailbox(ctx, config, uint32(uidRaw))
	if err != nil {
		return mcpgw.BuildToolErrorResult(err.Error()), nil
	}

	return mcpgw.BuildToolSuccessResult(map[string]any{
		"uid":         item.MessageID,
		"from":        item.From,
		"to":          item.To,
		"subject":     item.Subject,
		"body":        item.BodyText,
		"received_at": item.ReceivedAt,
	}), nil
}
