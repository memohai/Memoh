package personalwechat

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/memohai/memoh/internal/channel"
)

// Adapter implements a Memoh channel backed by a Wechaty sidecar process.
type Adapter struct {
	logger *slog.Logger

	mu      sync.RWMutex
	clients map[string]*bridgeClient
}

type bridgeClient struct {
	cfgID  string
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
}

// NewAdapter creates a personal WeChat adapter.
func NewAdapter(log *slog.Logger) *Adapter {
	if log == nil {
		log = slog.Default()
	}
	return &Adapter{
		logger:  log.With(slog.String("adapter", Type.String())),
		clients: make(map[string]*bridgeClient),
	}
}

func (*Adapter) Type() channel.ChannelType { return Type }

func (*Adapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{
		Type:        Type,
		DisplayName: "Personal WeChat",
		Capabilities: channel.ChannelCapabilities{
			Text:           true,
			Attachments:    true,
			Media:          true,
			Reply:          true,
			BlockStreaming: true,
			ChatTypes:      []string{channel.ConversationTypePrivate, channel.ConversationTypeGroup},
		},
		OutboundPolicy: channel.OutboundPolicy{
			TextChunkLimit:      1200,
			MediaOrder:          channel.OutboundOrderTextFirst,
			InlineTextWithMedia: true,
		},
		ConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields: map[string]channel.FieldSchema{
				"bridgeExecutable":     {Type: channel.FieldString, Title: "Bridge Executable", Description: "Executable used to launch the Wechaty sidecar. Default: node", Example: "node", Order: 0},
				"bridgeScript":         {Type: channel.FieldString, Title: "Bridge Script", Description: "Path to packages/personal-wechat-bridge/bin/personal-wechat-bridge.mjs or a deployed equivalent.", Example: defaultBridgeScript, Order: 10},
				"bridgeArgs":           {Type: channel.FieldString, Title: "Bridge Args", Description: "Optional whitespace-separated arguments passed after bridgeScript.", Order: 20},
				"dataDir":              {Type: channel.FieldString, Title: "Data Directory", Description: "Persistent directory for Wechaty memory card and diagnostics.", Example: defaultDataDir, Order: 30},
				"mediaDir":             {Type: channel.FieldString, Title: "Media Directory", Description: "Directory where inbound images/files are saved before Memoh ingests them.", Example: ".data/personal-wechat/media", Order: 40},
				"sessionName":          {Type: channel.FieldString, Title: "Session Name", Description: "Wechaty memory-card name.", Example: defaultSessionName, Order: 50},
				"botMentionName":       {Type: channel.FieldString, Title: "Bot Mention Name", Description: "Optional display mention that the sidecar can use for group filtering.", Order: 60},
				"allowPrivate":         {Type: channel.FieldBool, Title: "Allow Private Chats", Order: 70},
				"allowGroups":          {Type: channel.FieldBool, Title: "Allow Group Chats", Order: 80},
				"contactWhitelist":     {Type: channel.FieldString, Title: "Contact Whitelist", Description: "Comma-separated contact IDs or names. Empty allows all when allowPrivate is true.", Order: 90},
				"groupWhitelist":       {Type: channel.FieldString, Title: "Group Whitelist", Description: "Comma-separated room IDs or topics. Empty allows all when allowGroups is true.", Order: 100},
				"diagnosticRawPayload": {Type: channel.FieldBool, Title: "Diagnostic Raw Payload", Description: "Include sanitized raw Wechaty payload fields in controlled logs and inbound metadata.", Order: 110},
			},
		},
		UserConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields: map[string]channel.FieldSchema{
				"target":  {Type: channel.FieldString, Required: true, Title: "Target", Description: "contact:<wechat-id> or room:<room-id>"},
				"user_id": {Type: channel.FieldString, Title: "User ID"},
				"room_id": {Type: channel.FieldString, Title: "Room ID"},
			},
		},
		TargetSpec: channel.TargetSpec{
			Format: "contact:<wechat-id> | room:<room-id>",
			Hints: []channel.TargetHint{
				{Label: "Private", Example: "contact:wxid_xxx"},
				{Label: "Group", Example: "room:1234567890@chatroom"},
			},
		},
	}
}

func (*Adapter) NormalizeConfig(raw map[string]any) (map[string]any, error) {
	return normalizeConfig(raw)
}

func (*Adapter) NormalizeUserConfig(raw map[string]any) (map[string]any, error) {
	return normalizeUserConfig(raw)
}

func (*Adapter) NormalizeTarget(raw string) string { return normalizeTarget(raw) }

func (*Adapter) ResolveTarget(userConfig map[string]any) (string, error) {
	return resolveTarget(userConfig)
}

func (*Adapter) MatchBinding(config map[string]any, criteria channel.BindingCriteria) bool {
	return matchBinding(config, criteria)
}

func (*Adapter) BuildUserConfig(identity channel.Identity) map[string]any {
	return buildUserConfig(identity)
}

func (a *Adapter) Connect(ctx context.Context, cfg channel.ChannelConfig, handler channel.InboundHandler) (channel.Connection, error) {
	parsed, err := parseConfig(cfg.Credentials)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(parsed.DataDir, 0o700); err != nil {
		return nil, fmt.Errorf("personal_wechat create data dir: %w", err)
	}
	if err := os.MkdirAll(parsed.MediaDir, 0o700); err != nil {
		return nil, fmt.Errorf("personal_wechat create media dir: %w", err)
	}
	client, err := a.startBridge(ctx, cfg, parsed, handler)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	a.clients[cfg.ID] = client
	a.mu.Unlock()

	stop := func(stopCtx context.Context) error {
		a.mu.Lock()
		if current := a.clients[cfg.ID]; current == client {
			delete(a.clients, cfg.ID)
		}
		a.mu.Unlock()
		return client.stop(stopCtx)
	}
	return channel.NewConnection(cfg, stop), nil
}

func (a *Adapter) startBridge(ctx context.Context, cfg channel.ChannelConfig, parsed adapterConfig, handler channel.InboundHandler) (*bridgeClient, error) {
	bridgeCtx, cancel := context.WithCancel(ctx)
	args := []string{parsed.BridgeScript}
	args = append(args, strings.Fields(parsed.BridgeArgs)...)
	cmd := exec.CommandContext(bridgeCtx, parsed.BridgeExecutable, args...) //nolint:gosec // executable is explicit channel config.
	cmd.Dir = repoRootForBridge(parsed.BridgeScript)
	configPayload, err := json.Marshal(map[string]any{
		"configId":             cfg.ID,
		"botId":                cfg.BotID,
		"dataDir":              parsed.DataDir,
		"mediaDir":             parsed.MediaDir,
		"sessionName":          parsed.SessionName,
		"botMentionName":       parsed.BotMentionName,
		"allowPrivate":         parsed.AllowPrivate,
		"allowGroups":          parsed.AllowGroups,
		"contactWhitelist":     parsed.ContactWhitelist,
		"groupWhitelist":       parsed.GroupWhitelist,
		"diagnosticRawPayload": parsed.DiagnosticRawPayload,
	})
	if err != nil {
		cancel()
		return nil, err
	}
	cmd.Env = append(os.Environ(), "MEMOH_PERSONAL_WECHAT_CONFIG="+string(configPayload))

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("personal_wechat start bridge: %w", err)
	}
	client := &bridgeClient{cfgID: cfg.ID, cmd: cmd, stdin: stdin, cancel: cancel, done: make(chan struct{})}
	go a.readBridgeStdout(bridgeCtx, cfg, stdout, handler, client)
	go a.readBridgeStderr(stderr)
	go func() {
		err := cmd.Wait()
		if err != nil && bridgeCtx.Err() == nil && a.logger != nil {
			a.logger.Error("personal_wechat bridge exited", slog.String("config_id", cfg.ID), slog.Any("error", err))
		}
		close(client.done)
	}()
	return client, nil
}

func repoRootForBridge(script string) string {
	clean := filepath.Clean(script)
	if filepath.IsAbs(clean) {
		return ""
	}
	if strings.HasPrefix(clean, "packages"+string(filepath.Separator)) {
		return "."
	}
	return ""
}

func (a *Adapter) readBridgeStdout(ctx context.Context, cfg channel.ChannelConfig, r io.Reader, handler channel.InboundHandler, client *bridgeClient) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var env bridgeEnvelope
		if err := json.Unmarshal([]byte(line), &env); err != nil {
			if a.logger != nil {
				a.logger.Warn("personal_wechat bridge emitted non-json line", slog.String("line", line), slog.Any("error", err))
			}
			continue
		}
		switch env.Type {
		case "ready", "status":
			if a.logger != nil {
				a.logger.Info("personal_wechat bridge status", slog.String("config_id", cfg.ID), slog.String("status", env.Status))
			}
		case "message":
			if env.Message == nil {
				continue
			}
			inbound, ok := buildInboundMessage(*env.Message)
			if !ok {
				continue
			}
			inbound.BotID = cfg.BotID
			if err := handler(ctx, cfg, inbound); err != nil && a.logger != nil {
				a.logger.Error("personal_wechat inbound handler failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
			}
		case "error":
			if a.logger != nil {
				a.logger.Error("personal_wechat bridge error", slog.String("config_id", cfg.ID), slog.String("error", env.Error))
			}
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil && a.logger != nil {
		a.logger.Error("personal_wechat bridge stdout failed", slog.String("config_id", client.cfgID), slog.Any("error", err))
	}
}

func (a *Adapter) readBridgeStderr(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && a.logger != nil {
			a.logger.Info("personal_wechat bridge log", slog.String("line", line))
		}
	}
}

func (c *bridgeClient) stop(ctx context.Context) error {
	_ = c.writeJSON(bridgeStopCommand{Type: "stop"})
	c.cancel()
	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		if c.cmd != nil && c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		return nil
	}
}

func (c *bridgeClient) writeJSON(value any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stdin == nil {
		return errors.New("personal_wechat bridge stdin is closed")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	_, err = c.stdin.Write(payload)
	return err
}

func (a *Adapter) Send(ctx context.Context, cfg channel.ChannelConfig, msg channel.PreparedOutboundMessage) error {
	target := strings.TrimSpace(msg.Target)
	if target == "" {
		return errors.New("personal_wechat target is required")
	}
	logical := msg.Message.LogicalMessage()
	if logical.IsEmpty() {
		return errors.New("personal_wechat message is required")
	}
	a.mu.RLock()
	client := a.clients[cfg.ID]
	a.mu.RUnlock()
	if client == nil {
		return errors.New("personal_wechat bridge is not connected")
	}
	cmd := bridgeSendCommand{
		Type:   "send",
		Target: normalizeTarget(target),
		Message: bridgeSendMessage{
			Text:   logical.Text,
			Reply:  logical.Reply,
			Format: string(logical.Format),
		},
	}
	for _, att := range logical.Attachments {
		cmd.Message.Attachments = append(cmd.Message.Attachments, bridgeAttachment{
			Type: string(att.Type),
			Path: att.Path,
			URL:  att.URL,
			Mime: att.Mime,
			Name: att.Name,
			Size: att.Size,
		})
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return client.writeJSON(cmd)
	}
}
