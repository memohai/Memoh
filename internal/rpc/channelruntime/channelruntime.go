package channelruntime

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/email"
	runtimeRpc "github.com/memohai/memoh/internal/rpc/runtime"
	"github.com/memohai/memoh/internal/webhooktunnel"
)

const (
	MethodUpsertConfig = "channel.config.upsert"
	MethodSetStatus    = "channel.config.status"
	MethodDeleteConfig = "channel.config.delete"
	MethodSetWebhook   = "channel.webhook.set"
	MethodSend         = "channel.message.send"
	MethodReact        = "channel.message.react"
	MethodStatuses     = "channel.connection.statuses"
	MethodRefreshEmail = "channel.email.refresh"
	MethodSendEmail    = "channel.email.send"
	MethodTunnelStatus = "channel.tunnel.status"

	reasonConfigNotFound     = "channel.config_not_found"
	reasonDiscoveryFailed    = "channel.discovery_failed"
	reasonEnableFailed       = "channel.enable_failed"
	reasonInvalidWebhook     = "channel.invalid_webhook"
	reasonWebhookUnsupported = "channel.webhook_unsupported"
)

type Client struct{ rpc *runtimeRpc.Client }

func NewClient(rpc *runtimeRpc.Client) *Client { return &Client{rpc: rpc} }

type channelInput struct {
	BotID       string
	ChannelType channel.ChannelType
	Config      channel.UpsertConfigRequest
	Disabled    bool
	Webhook     channel.SetWebhookEndpointRequest
	Send        channel.SendRequest
	React       channel.ReactRequest
}

func (c *Client) UpsertBotChannelConfig(ctx context.Context, botID string, typ channel.ChannelType, req channel.UpsertConfigRequest) (channel.ChannelConfig, error) {
	var out channel.ChannelConfig
	return out, c.call(ctx, MethodUpsertConfig, channelInput{BotID: botID, ChannelType: typ, Config: req}, &out)
}

func (c *Client) SetBotChannelStatus(ctx context.Context, botID string, typ channel.ChannelType, disabled bool) (channel.ChannelConfig, error) {
	var out channel.ChannelConfig
	return out, c.call(ctx, MethodSetStatus, channelInput{BotID: botID, ChannelType: typ, Disabled: disabled}, &out)
}

func (c *Client) DeleteBotChannelConfig(ctx context.Context, botID string, typ channel.ChannelType) error {
	return c.call(ctx, MethodDeleteConfig, channelInput{BotID: botID, ChannelType: typ}, nil)
}

func (c *Client) SetWebhookEndpoint(ctx context.Context, botID string, typ channel.ChannelType, req channel.SetWebhookEndpointRequest) (channel.SetWebhookEndpointResponse, error) {
	var out channel.SetWebhookEndpointResponse
	return out, c.call(ctx, MethodSetWebhook, channelInput{BotID: botID, ChannelType: typ, Webhook: req}, &out)
}

func (c *Client) Send(ctx context.Context, botID string, typ channel.ChannelType, req channel.SendRequest) error {
	return c.call(ctx, MethodSend, channelInput{BotID: botID, ChannelType: typ, Send: req}, nil)
}

func (c *Client) React(ctx context.Context, botID string, typ channel.ChannelType, req channel.ReactRequest) error {
	return c.call(ctx, MethodReact, channelInput{BotID: botID, ChannelType: typ, React: req}, nil)
}

func (c *Client) ConnectionStatusesByBot(botID string) []channel.ConnectionStatus {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out []channel.ConnectionStatus
	if c.call(ctx, MethodStatuses, botID, &out) != nil {
		return nil
	}
	return out
}

func (c *Client) RefreshProvider(ctx context.Context, providerID string) error {
	return c.call(ctx, MethodRefreshEmail, providerID, nil)
}

func (c *Client) SendEmail(ctx context.Context, botID, providerID string, msg email.OutboundEmail) (string, error) {
	in := struct {
		BotID, ProviderID string
		Message           email.OutboundEmail
	}{botID, providerID, msg}
	var out string
	return out, c.call(ctx, MethodSendEmail, in, &out)
}

func (c *Client) Status() webhooktunnel.Status {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out webhooktunnel.Status
	if c.call(ctx, MethodTunnelStatus, nil, &out) != nil {
		return webhooktunnel.Status{Enabled: false, Mode: "unavailable", Status: webhooktunnel.StatusError}
	}
	return out
}

func (c *Client) call(ctx context.Context, method string, input, output any) error {
	err := c.rpc.Call(ctx, method, input, output)
	if err == nil || errors.Is(err, runtimeRpc.ErrUnavailable) {
		return err
	}
	switch status.Convert(err).Message() {
	case reasonConfigNotFound:
		return channel.ErrChannelConfigNotFound
	case reasonDiscoveryFailed:
		return channel.ErrChannelDiscoveryFailed
	case reasonEnableFailed:
		return channel.ErrEnableChannelFailed
	case reasonInvalidWebhook:
		return channel.ErrInvalidWebhookEndpoint
	case reasonWebhookUnsupported:
		return channel.ErrWebhookEndpointUnsupported
	default:
		return err
	}
}

func safeChannelError(err error) error {
	switch {
	case errors.Is(err, channel.ErrChannelConfigNotFound):
		return status.Error(codes.NotFound, reasonConfigNotFound)
	case errors.Is(err, channel.ErrChannelDiscoveryFailed):
		return status.Error(codes.FailedPrecondition, reasonDiscoveryFailed)
	case errors.Is(err, channel.ErrEnableChannelFailed):
		return status.Error(codes.FailedPrecondition, reasonEnableFailed)
	case errors.Is(err, channel.ErrInvalidWebhookEndpoint):
		return status.Error(codes.InvalidArgument, reasonInvalidWebhook)
	case errors.Is(err, channel.ErrWebhookEndpointUnsupported):
		return status.Error(codes.Unimplemented, reasonWebhookUnsupported)
	default:
		return err
	}
}

func Handlers(channelRuntime channel.Runtime, emailRuntime email.Runtime, tunnel *webhooktunnel.Manager) map[string]runtimeRpc.Handler {
	decode := func(raw json.RawMessage, dst any) error { return json.Unmarshal(raw, dst) }
	return map[string]runtimeRpc.Handler{
		MethodUpsertConfig: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in channelInput
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			out, err := channelRuntime.UpsertBotChannelConfig(ctx, in.BotID, in.ChannelType, in.Config)
			return out, safeChannelError(err)
		},
		MethodSetStatus: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in channelInput
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			out, err := channelRuntime.SetBotChannelStatus(ctx, in.BotID, in.ChannelType, in.Disabled)
			return out, safeChannelError(err)
		},
		MethodDeleteConfig: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in channelInput
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			return nil, safeChannelError(channelRuntime.DeleteBotChannelConfig(ctx, in.BotID, in.ChannelType))
		},
		MethodSetWebhook: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in channelInput
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			out, err := channelRuntime.SetWebhookEndpoint(ctx, in.BotID, in.ChannelType, in.Webhook)
			return out, safeChannelError(err)
		},
		MethodSend: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in channelInput
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			return nil, channelRuntime.Send(ctx, in.BotID, in.ChannelType, in.Send)
		},
		MethodReact: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in channelInput
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			return nil, channelRuntime.React(ctx, in.BotID, in.ChannelType, in.React)
		},
		MethodStatuses: func(_ context.Context, raw json.RawMessage) (any, error) {
			var botID string
			if err := decode(raw, &botID); err != nil {
				return nil, err
			}
			return channelRuntime.ConnectionStatusesByBot(botID), nil
		},
		MethodRefreshEmail: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var id string
			if err := decode(raw, &id); err != nil {
				return nil, err
			}
			return nil, emailRuntime.RefreshProvider(ctx, id)
		},
		MethodSendEmail: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var in struct {
				BotID, ProviderID string
				Message           email.OutboundEmail
			}
			if err := decode(raw, &in); err != nil {
				return nil, err
			}
			return emailRuntime.SendEmail(ctx, in.BotID, in.ProviderID, in.Message)
		},
		MethodTunnelStatus: func(context.Context, json.RawMessage) (any, error) { return tunnel.Status(), nil },
	}
}

var (
	_ channel.Runtime = (*Client)(nil)
	_ email.Runtime   = (*Client)(nil)
)
