package flow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/heartbeat"
	"github.com/memohai/memoh/internal/schedule"
	sdk "github.com/memohai/twilight-ai/sdk"
)

// TriggerSchedule executes a scheduled command via the internal agent.
func (r *Resolver) TriggerSchedule(ctx context.Context, botID string, payload schedule.TriggerPayload, token string) error {
	if strings.TrimSpace(botID) == "" {
		return errors.New("bot id is required")
	}
	if strings.TrimSpace(payload.Command) == "" {
		return errors.New("schedule command is required")
	}

	req := conversation.ChatRequest{
		BotID:  botID,
		ChatID: botID,
		Query:  payload.Command,
		UserID: payload.OwnerUserID,
		Token:  token,
	}
	rc, err := r.resolve(ctx, req)
	if err != nil {
		return err
	}

	cfg := rc.runConfig
	cfg.Identity.ChannelIdentityID = strings.TrimSpace(payload.OwnerUserID)
	cfg.Identity.DisplayName = "Scheduler"

	schedulePrompt := agentpkg.GenerateSchedulePrompt(agentpkg.Schedule{
		ID:          payload.ID,
		Name:        payload.Name,
		Description: payload.Description,
		Pattern:     payload.Pattern,
		MaxCalls:    payload.MaxCalls,
		Command:     payload.Command,
	})
	cfg.Messages = append(cfg.Messages, sdk.UserMessage(schedulePrompt))
	cfg = r.prepareRunConfig(ctx, cfg)

	result, err := r.agent.Generate(ctx, cfg)
	if err != nil {
		return err
	}

	outputMessages := sdkMessagesToModelMessages(result.Messages)
	roundMessages := prependUserMessage(req.Query, outputMessages)
	usageJSON, _ := json.Marshal(result.Usage)
	return r.storeRound(ctx, req, roundMessages, usageJSON, nil, rc.model.ID)
}

// TriggerHeartbeat executes a heartbeat check via the internal agent.
func (r *Resolver) TriggerHeartbeat(ctx context.Context, botID string, payload heartbeat.TriggerPayload, token string) (heartbeat.TriggerResult, error) {
	if strings.TrimSpace(botID) == "" {
		return heartbeat.TriggerResult{}, errors.New("bot id is required")
	}

	var heartbeatModel string
	if botSettings, err := r.loadBotSettings(ctx, botID); err == nil {
		heartbeatModel = strings.TrimSpace(botSettings.HeartbeatModelID)
	}

	req := conversation.ChatRequest{
		BotID:  botID,
		ChatID: botID,
		Query:  "heartbeat",
		UserID: payload.OwnerUserID,
		Token:  token,
		Model:  heartbeatModel,
	}
	rc, err := r.resolve(ctx, req)
	if err != nil {
		return heartbeat.TriggerResult{}, err
	}

	cfg := rc.runConfig
	cfg.Identity.ChannelIdentityID = strings.TrimSpace(payload.OwnerUserID)
	cfg.Identity.DisplayName = "Heartbeat"

	var checklist string
	if r.agent != nil {
		fs := agentpkg.NewFSClient(nil, botID)
		checklist = fs.ReadTextSafe(ctx, "/data/HEARTBEAT.md")
	}
	heartbeatPrompt := agentpkg.GenerateHeartbeatPrompt(payload.Interval, checklist)
	cfg.Messages = append(cfg.Messages, sdk.UserMessage(heartbeatPrompt))
	cfg = r.prepareRunConfig(ctx, cfg)

	result, err := r.agent.Generate(ctx, cfg)
	if err != nil {
		return heartbeat.TriggerResult{}, err
	}

	status := "alert"
	text := strings.TrimSpace(result.Text)
	if isHeartbeatOK(text) {
		status = "ok"
	}

	usageJSON, _ := json.Marshal(result.Usage)

	return heartbeat.TriggerResult{
		Status:     status,
		Text:       text,
		Usage:      usageJSON,
		UsageBytes: usageJSON,
		ModelID:    rc.model.ID,
	}, nil
}

func isHeartbeatOK(text string) bool {
	t := strings.TrimSpace(text)
	return strings.HasPrefix(t, "HEARTBEAT_OK") || strings.HasSuffix(t, "HEARTBEAT_OK") || t == "HEARTBEAT_OK"
}
