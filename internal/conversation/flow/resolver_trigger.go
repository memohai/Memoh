package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/heartbeat"
	"github.com/memohai/memoh/internal/schedule"
)

// RouteService is the interface the resolver uses to recover route-backed
// delivery context for proactive background notifications.
type RouteService interface {
	GetByID(ctx context.Context, routeID string) (route.Route, error)
}

// SetRouteService configures the route service used for background delivery
// context resolution.
func (r *Resolver) SetRouteService(s RouteService) {
	r.routeService = s
}

// TriggerSchedule executes a scheduled command via the internal agent.
func (r *Resolver) TriggerSchedule(ctx context.Context, botID string, payload schedule.TriggerPayload, token string) (schedule.TriggerResult, error) {
	if strings.TrimSpace(botID) == "" {
		return schedule.TriggerResult{}, errors.New("bot id is required")
	}
	if strings.TrimSpace(payload.Command) == "" {
		return schedule.TriggerResult{}, errors.New("schedule command is required")
	}

	req := conversation.ChatRequest{
		BotID:     botID,
		ChatID:    botID,
		SessionID: payload.SessionID,
		Query:     payload.Command,
		UserID:    payload.OwnerUserID,
		Token:     token,
	}
	rc, err := r.resolve(ctx, req)
	if err != nil {
		return schedule.TriggerResult{}, err
	}

	cfg := rc.runConfig
	cfg.SessionType = "schedule"
	cfg.Identity.ChannelIdentityID = strings.TrimSpace(payload.OwnerUserID)

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
		return schedule.TriggerResult{}, err
	}

	outputMessages := sdkMessagesToModelMessages(result.Messages)
	roundMessages := prependUserMessage(req.Query, outputMessages)
	storeErr := r.storeRound(ctx, req, roundMessages, rc.model.ID)

	totalUsageJSON, _ := json.Marshal(result.Usage)
	return schedule.TriggerResult{
		Status:     "ok",
		Text:       strings.TrimSpace(result.Text),
		UsageBytes: totalUsageJSON,
		ModelID:    rc.model.ID,
	}, storeErr
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
		BotID:     botID,
		ChatID:    botID,
		SessionID: payload.SessionID,
		Query:     "heartbeat",
		UserID:    payload.OwnerUserID,
		Token:     token,
		Model:     heartbeatModel,
	}
	rc, err := r.resolve(ctx, req)
	if err != nil {
		return heartbeat.TriggerResult{}, err
	}

	cfg := rc.runConfig
	cfg.SessionType = "heartbeat"
	cfg.Identity.ChannelIdentityID = strings.TrimSpace(payload.OwnerUserID)

	var checklist string
	if r.agent != nil {
		nowFn := time.Now
		if cfg.Identity.TimezoneLocation != nil {
			nowFn = func() time.Time { return time.Now().In(cfg.Identity.TimezoneLocation) }
		}
		fs := agentpkg.NewFSClient(r.agent.BridgeProvider(), botID, nowFn)
		checklist = fs.ReadTextSafe(ctx, "/data/HEARTBEAT.md")
	}
	now := time.Now().UTC()
	if cfg.Identity.TimezoneLocation != nil {
		now = now.In(cfg.Identity.TimezoneLocation)
	}
	heartbeatPrompt := agentpkg.GenerateHeartbeatPrompt(payload.Interval, checklist, now, payload.LastHeartbeatAt)
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

	outputMessages := sdkMessagesToModelMessages(result.Messages)
	roundMessages := prependUserMessage(heartbeatPrompt, outputMessages)
	_ = r.storeRound(ctx, req, roundMessages, rc.model.ID)

	totalUsageJSON, _ := json.Marshal(result.Usage)
	return heartbeat.TriggerResult{
		Status:     status,
		Text:       text,
		Usage:      totalUsageJSON,
		UsageBytes: totalUsageJSON,
		ModelID:    rc.model.ID,
		SessionID:  payload.SessionID,
	}, nil
}

func isHeartbeatOK(text string) bool {
	t := strings.TrimSpace(text)
	return strings.HasPrefix(t, "HEARTBEAT_OK") || strings.HasSuffix(t, "HEARTBEAT_OK") || t == "HEARTBEAT_OK"
}

type backgroundDeliveryContext struct {
	routeID     string
	channelType string
	replyTarget string
}

// TriggerBackgroundNotification is called when background-task notifications
// are enqueued for a session. Delivery is session-centric: all pending
// notifications for a session are drained together and delivered using the
// current session/route delivery context. It only runs when the session is
// currently idle; active turns consume notifications via mid-turn drain.
func (r *Resolver) TriggerBackgroundNotification(ctx context.Context, botID, sessionID string) {
	r.logger.Info("background notification trigger called",
		slog.String("bot_id", botID),
		slog.String("session_id", sessionID),
	)
	if strings.TrimSpace(botID) == "" || strings.TrimSpace(sessionID) == "" {
		return
	}
	if r.bgManager == nil {
		return
	}
	if !r.bgManager.HasNotifications(botID, sessionID) {
		return
	}
	doneTurn, ok := r.tryEnterIdleSessionTurn(ctx, botID, sessionID)
	if !ok {
		r.markDeferredBackgroundNotification(botID, sessionID)
		r.logger.Info("background notification trigger deferred: session turn active",
			slog.String("bot_id", botID),
			slog.String("session_id", sessionID),
		)
		return
	}
	defer doneTurn()

	notifications := r.bgManager.DrainNotifications(botID, sessionID)
	if len(notifications) == 0 {
		return
	}

	notifMessages := make([]sdk.Message, 0, len(notifications))
	for _, n := range notifications {
		notifMessages = append(notifMessages, sdk.UserMessage(n.MessageText()))
	}

	delivery, err := r.resolveBackgroundDeliveryContext(ctx, botID, sessionID)
	if err != nil {
		r.bgManager.RequeueNotifications(notifications)
		r.logger.Warn("background notification trigger: resolve delivery context failed",
			slog.String("bot_id", botID),
			slog.String("session_id", sessionID),
			slog.Any("error", err),
		)
		return
	}

	if err := r.deliverBackgroundNotifications(ctx, botID, sessionID, delivery, notifMessages); err != nil {
		r.bgManager.RequeueNotifications(notifications)
		r.logger.Warn("background notification trigger: deliver failed",
			slog.String("bot_id", botID),
			slog.String("session_id", sessionID),
			slog.Any("error", err),
		)
	}
}

func (r *Resolver) resolveBackgroundDeliveryContext(ctx context.Context, botID, sessionID string) (backgroundDeliveryContext, error) {
	if r.sessionService == nil {
		return backgroundDeliveryContext{}, errors.New("session service not configured")
	}

	sess, err := r.sessionService.Get(ctx, sessionID)
	if err != nil {
		return backgroundDeliveryContext{}, fmt.Errorf("get session: %w", err)
	}
	if sess.BotID != "" && botID != "" && sess.BotID != botID {
		return backgroundDeliveryContext{}, fmt.Errorf("session %s belongs to bot %s, not %s", sessionID, sess.BotID, botID)
	}

	channelType := strings.TrimSpace(sess.ChannelType)
	if routeID := strings.TrimSpace(sess.RouteID); routeID != "" {
		if r.routeService == nil {
			return backgroundDeliveryContext{}, errors.New("route service not configured")
		}
		rt, err := r.routeService.GetByID(ctx, routeID)
		if err != nil {
			return backgroundDeliveryContext{}, fmt.Errorf("get route: %w", err)
		}
		if channelType == "" {
			channelType = strings.TrimSpace(rt.Platform)
		}
		return backgroundDeliveryContext{
			routeID:     routeID,
			channelType: channelType,
			replyTarget: strings.TrimSpace(rt.ReplyTarget),
		}, nil
	}

	if strings.EqualFold(channelType, "local") {
		return backgroundDeliveryContext{
			channelType: "local",
			replyTarget: botID,
		}, nil
	}

	return backgroundDeliveryContext{}, fmt.Errorf("session %s has no route-backed delivery context", sessionID)
}

// deliverBackgroundNotifications runs a single agent call to deliver a batch of
// background-task notifications to the session's current delivery context.
func (r *Resolver) deliverBackgroundNotifications(ctx context.Context, botID, sessionID string, delivery backgroundDeliveryContext, notifMessages []sdk.Message) error {
	r.logger.Info("background notification delivery",
		slog.String("bot_id", botID),
		slog.String("session_id", sessionID),
		slog.String("route_id", delivery.routeID),
		slog.String("platform", delivery.channelType),
		slog.String("reply_target", delivery.replyTarget),
		slog.Int("count", len(notifMessages)),
	)
	req := conversation.ChatRequest{
		BotID:          botID,
		ChatID:         botID,
		SessionID:      sessionID,
		RouteID:        delivery.routeID,
		Query:          "[background notification]",
		CurrentChannel: delivery.channelType,
		ReplyTarget:    delivery.replyTarget,
	}
	rc, err := r.resolve(ctx, req)
	if err != nil {
		return fmt.Errorf("resolve background delivery: %w", err)
	}

	cfg := rc.runConfig
	// Inject drained notifications so the first LLM call sees them.
	cfg.Messages = append(cfg.Messages, notifMessages...)
	// Clear query so prepareRunConfig does not append a redundant user message.
	cfg.Query = ""
	// Use the natural session type — same system prompt, same tools, same
	// personality as a regular conversation turn. Between-turn notifications
	// should go through the same execution path as normal user messages.
	cfg = r.prepareRunConfig(ctx, cfg)

	result, err := r.agent.Generate(ctx, cfg)
	if err != nil {
		return fmt.Errorf("generate background delivery: %w", err)
	}
	r.logger.Info("background notification trigger: generate ok",
		slog.String("bot_id", botID),
		slog.String("platform", delivery.channelType),
		slog.String("reply_target", delivery.replyTarget),
		slog.Int("messages", len(result.Messages)),
	)

	if len(result.Messages) > 0 {
		outputMessages := sdkMessagesToModelMessages(result.Messages)
		notifModelMessages := sdkMessagesToModelMessages(notifMessages)
		roundMessages := append(append(make([]conversation.ModelMessage, 0, len(notifModelMessages)+len(outputMessages)), notifModelMessages...), outputMessages...)
		_ = r.storeRound(ctx, req, roundMessages, rc.model.ID)
	}

	// Auto-deliver the agent's text response to the user through the normal
	// outbound path, not through a special "send" tool call.
	if text := strings.TrimSpace(result.Text); text != "" && r.outboundFn != nil {
		if err := r.outboundFn(ctx, botID, delivery.channelType, delivery.replyTarget, text); err != nil {
			r.logger.Warn("background notification: outbound delivery failed",
				slog.String("bot_id", botID),
				slog.String("platform", delivery.channelType),
				slog.String("reply_target", delivery.replyTarget),
				slog.Any("error", err),
			)
		}
	}
	return nil
}
