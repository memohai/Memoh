package inbound

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/bind"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/identities"
	"github.com/memohai/memoh/internal/preauth"
)

// IdentityDecision indicates whether the inbound message should be stopped with an optional reply.
type IdentityDecision struct {
	Stop  bool
	Reply channel.Message
}

// Identity carries the resolved channel identity for an inbound message.
type Identity struct {
	BotID             string
	ChannelConfigID   string
	SubjectID         string
	ChannelIdentityID string
	UserID            string
	DisplayName       string
	AvatarURL         string
	ForceReply        bool
}

// IdentityState bundles resolved identity with an optional early-exit decision.
type IdentityState struct {
	Identity Identity
	Decision *IdentityDecision
}

type identityContextKey struct{}

// WithIdentityState stores IdentityState in the context.
func WithIdentityState(ctx context.Context, state IdentityState) context.Context {
	return context.WithValue(ctx, identityContextKey{}, state)
}

// IdentityStateFromContext retrieves IdentityState from the context.
func IdentityStateFromContext(ctx context.Context) (IdentityState, bool) {
	if ctx == nil {
		return IdentityState{}, false
	}
	raw := ctx.Value(identityContextKey{})
	if raw == nil {
		return IdentityState{}, false
	}
	state, ok := raw.(IdentityState)
	return state, ok
}

// ChannelIdentityService is the minimal interface for channel identity resolution.
type ChannelIdentityService interface {
	ResolveByChannelIdentity(ctx context.Context, channel, channelSubjectID, displayName string, meta map[string]any) (identities.ChannelIdentity, error)
	Canonicalize(ctx context.Context, channelIdentityID string) (string, error)
	GetLinkedUserID(ctx context.Context, channelIdentityID string) (string, error)
	LinkChannelIdentityToUser(ctx context.Context, channelIdentityID, userID string) error
}

// BotMemberService checks and manages bot membership.
type BotMemberService interface {
	IsMember(ctx context.Context, botID, channelIdentityID string) (bool, error)
	UpsertMemberSimple(ctx context.Context, botID, channelIdentityID, role string) error
}

// PolicyService resolves access policy for a bot.
type PolicyService interface {
	AllowGuest(ctx context.Context, botID string) (bool, error)
	BotType(ctx context.Context, botID string) (string, error)
	BotOwnerUserID(ctx context.Context, botID string) (string, error)
}

// PreauthService handles preauth key validation.
type PreauthService interface {
	Get(ctx context.Context, token string) (preauth.Key, error)
	MarkUsed(ctx context.Context, id string) (preauth.Key, error)
}

// BindService handles channel identity bind code validation and consumption.
type BindService interface {
	Get(ctx context.Context, token string) (bind.Code, error)
	Consume(ctx context.Context, code bind.Code, channelIdentityID string) error
}

// IdentityResolver implements identity resolution with bind code, preauth, and guest fallback.
type IdentityResolver struct {
	registry          *channel.Registry
	channelIdentities ChannelIdentityService
	members           BotMemberService
	policy            PolicyService
	preauth           PreauthService
	bind              BindService
	logger            *slog.Logger
	unboundReply      string
	preauthReply      string
	bindReply         string
}

// NewIdentityResolver creates an IdentityResolver.
func NewIdentityResolver(
	log *slog.Logger,
	registry *channel.Registry,
	channelIdentityService ChannelIdentityService,
	memberService BotMemberService,
	policyService PolicyService,
	preauthService PreauthService,
	bindService BindService,
	unboundReply, preauthReply string,
) *IdentityResolver {
	if log == nil {
		log = slog.Default()
	}
	if strings.TrimSpace(unboundReply) == "" {
		unboundReply = "Access denied. Please contact the administrator."
	}
	if strings.TrimSpace(preauthReply) == "" {
		preauthReply = "Authorization successful."
	}
	return &IdentityResolver{
		registry:          registry,
		channelIdentities: channelIdentityService,
		members:           memberService,
		policy:            policyService,
		preauth:           preauthService,
		bind:              bindService,
		logger:            log.With(slog.String("component", "channel_identity")),
		unboundReply:      unboundReply,
		preauthReply:      preauthReply,
		bindReply:         "Binding successful! Your identity has been linked.",
	}
}

// Middleware returns a channel middleware that resolves identity before processing.
func (r *IdentityResolver) Middleware() channel.Middleware {
	return func(next channel.InboundHandler) channel.InboundHandler {
		return func(ctx context.Context, cfg channel.Config, msg channel.InboundMessage) error {
			state, err := r.Resolve(ctx, cfg, msg)
			if err != nil {
				return err
			}
			return next(WithIdentityState(ctx, state), cfg, msg)
		}
	}
}

// Resolve performs two-phase identity resolution:
//  1. Global identity: (channel, channel_subject_id) -> channel_identity_id (unconditional)
//  2. Authorization: bot membership check with guest/preauth fallback
func (r *IdentityResolver) Resolve(ctx context.Context, cfg channel.Config, msg channel.InboundMessage) (IdentityState, error) {
	if r.channelIdentities == nil {
		return IdentityState{}, errors.New("identity resolver not configured")
	}

	botID := strings.TrimSpace(msg.BotID)
	if botID == "" {
		botID = cfg.BotID
	}

	channelConfigID := cfg.ID
	if r.registry != nil && r.registry.IsConfigless(msg.Channel) {
		channelConfigID = ""
	}
	subjectID := extractSubjectIdentity(msg)
	displayName, avatarURL := r.resolveProfile(ctx, cfg, msg, subjectID)

	state := IdentityState{
		Identity: Identity{
			BotID:           botID,
			ChannelConfigID: channelConfigID,
			SubjectID:       subjectID,
		},
	}

	// Phase 1: Global identity resolution (unconditional).
	if subjectID == "" {
		return state, errors.New("cannot resolve identity: no channel_subject_id")
	}

	channelIdentityID, linkedUserID, err := r.resolveIdentityWithLinkedUser(ctx, msg, subjectID, displayName, avatarURL)
	if err != nil {
		return state, err
	}
	state.Identity.ChannelIdentityID = channelIdentityID
	state.Identity.UserID = strings.TrimSpace(linkedUserID)
	if strings.TrimSpace(state.Identity.UserID) == "" {
		state.Identity.UserID = r.tryLinkConfiglessChannelIdentityToUser(ctx, msg, channelIdentityID)
	}
	state.Identity.DisplayName = displayName
	state.Identity.AvatarURL = avatarURL

	// Bind code check runs before membership/guest checks so linking is always reachable.
	if handled, decision, newUserID, err := r.tryHandleBindCode(ctx, msg, channelIdentityID, subjectID); handled {
		if strings.TrimSpace(newUserID) != "" {
			state.Identity.UserID = strings.TrimSpace(newUserID)
		}
		state.Decision = &decision
		return state, err
	}

	// Personal bots are owner-only and must not depend on member/guest/preauth bypass.
	if r.policy != nil {
		botType, err := r.policy.BotType(ctx, botID)
		if err != nil {
			return state, err
		}
		if strings.EqualFold(strings.TrimSpace(botType), "personal") {
			ownerUserID, err := r.policy.BotOwnerUserID(ctx, botID)
			if err != nil {
				return state, err
			}
			isOwner := strings.TrimSpace(state.Identity.UserID) != "" &&
				strings.TrimSpace(ownerUserID) == strings.TrimSpace(state.Identity.UserID)
			if !isOwner {
				// Ignore all non-owner messages for personal bots.
				state.Decision = &IdentityDecision{Stop: true}
				return state, nil
			}
			// Owner is authorized, but group trigger policy is still decided by
			// shouldTriggerAssistantResponse in channel routing.
			return state, nil
		}
	}

	// Phase 2: Authorization (bot membership check).
	if r.members != nil {
		if strings.TrimSpace(state.Identity.UserID) != "" {
			isMember, err := r.members.IsMember(ctx, botID, state.Identity.UserID)
			if err != nil {
				return state, fmt.Errorf("check bot membership: %w", err)
			}
			if isMember {
				return state, nil
			}
		}
	}
	if r.policy != nil && strings.TrimSpace(state.Identity.UserID) != "" {
		ownerUserID, err := r.policy.BotOwnerUserID(ctx, botID)
		if err != nil {
			return state, err
		}
		// Bot owner should not depend on bot_members linkage.
		if strings.TrimSpace(ownerUserID) == strings.TrimSpace(state.Identity.UserID) {
			return state, nil
		}
	}

	// Guest policy check.
	if r.policy != nil {
		allowed, err := r.policy.AllowGuest(ctx, botID)
		if err != nil {
			return state, err
		}
		if allowed {
			return state, nil
		}
	}

	// Preauth key check.
	if handled, decision, err := r.tryHandlePreauthKey(ctx, msg, botID, state.Identity.UserID, subjectID); handled {
		state.Decision = &decision
		return state, err
	}

	// In group conversations, silently drop unauthorized messages to avoid spamming
	// the channel with "access denied" replies (same behavior as personal bot non-owner).
	if isGroupConversationType(msg.Conversation.Type) {
		state.Decision = &IdentityDecision{Stop: true}
		return state, nil
	}

	state.Decision = &IdentityDecision{
		Stop:  true,
		Reply: channel.Message{Text: r.unboundReply},
	}
	return state, nil
}

func (r *IdentityResolver) resolveIdentityWithLinkedUser(ctx context.Context, msg channel.InboundMessage, primarySubjectID, displayName, avatarURL string) (string, string, error) {
	candidates := identitySubjectCandidates(msg, primarySubjectID)
	if len(candidates) == 0 {
		return "", "", errors.New("cannot resolve identity: no channel_subject_id")
	}

	var meta map[string]any
	if strings.TrimSpace(avatarURL) != "" {
		meta = map[string]any{"avatar_url": strings.TrimSpace(avatarURL)}
	}

	firstChannelIdentityID := ""
	for _, subjectID := range candidates {
		channelIdentity, err := r.channelIdentities.ResolveByChannelIdentity(ctx, msg.Channel.String(), subjectID, displayName, meta)
		if err != nil {
			return "", "", fmt.Errorf("resolve channel identity: %w", err)
		}
		channelIdentityID := strings.TrimSpace(channelIdentity.ID)
		if channelIdentityID == "" {
			continue
		}
		if firstChannelIdentityID == "" {
			firstChannelIdentityID = channelIdentityID
		}
		linkedUserID, err := r.channelIdentities.GetLinkedUserID(ctx, channelIdentityID)
		if err != nil {
			return "", "", err
		}
		linkedUserID = strings.TrimSpace(linkedUserID)
		if linkedUserID != "" {
			return channelIdentityID, linkedUserID, nil
		}
	}
	return firstChannelIdentityID, "", nil
}

func (r *IdentityResolver) tryHandlePreauthKey(ctx context.Context, msg channel.InboundMessage, botID, userID, subjectID string) (bool, IdentityDecision, error) {
	tokenText := strings.TrimSpace(msg.Message.PlainText())
	if tokenText == "" || r.preauth == nil {
		return false, IdentityDecision{}, nil
	}
	key, err := r.preauth.Get(ctx, tokenText)
	if err != nil {
		if errors.Is(err, preauth.ErrKeyNotFound) {
			return false, IdentityDecision{}, nil
		}
		return true, IdentityDecision{}, err
	}
	reply := func(text string) IdentityDecision {
		return IdentityDecision{
			Stop:  true,
			Reply: channel.Message{Text: text},
		}
	}
	if !key.UsedAt.IsZero() {
		return true, reply("Preauth key already used."), nil
	}
	if !key.ExpiresAt.IsZero() && time.Now().UTC().After(key.ExpiresAt) {
		return true, reply("Preauth key expired."), nil
	}
	if key.BotID != botID {
		return true, reply("Preauth key mismatch."), nil
	}
	if subjectID == "" {
		return true, reply("Cannot identify current account."), nil
	}

	// Grant membership via preauth.
	if strings.TrimSpace(userID) == "" {
		return true, reply("Current channel account is not linked to a user."), nil
	}
	if r.members != nil {
		if err := r.members.UpsertMemberSimple(ctx, botID, userID, "member"); err != nil {
			return true, IdentityDecision{}, fmt.Errorf("upsert preauth member: %w", err)
		}
	}
	if _, err := r.preauth.MarkUsed(ctx, key.ID); err != nil {
		return true, IdentityDecision{}, fmt.Errorf("mark preauth key used: %w", err)
	}
	return true, reply(r.preauthReply), nil
}

func (r *IdentityResolver) tryHandleBindCode(ctx context.Context, msg channel.InboundMessage, channelIdentityID, subjectID string) (bool, IdentityDecision, string, error) {
	tokenText := strings.TrimSpace(msg.Message.PlainText())
	if tokenText == "" || r.bind == nil {
		return false, IdentityDecision{}, "", nil
	}
	code, err := r.bind.Get(ctx, tokenText)
	if err != nil {
		if errors.Is(err, bind.ErrCodeNotFound) {
			return false, IdentityDecision{}, "", nil
		}
		return true, IdentityDecision{}, "", err
	}
	reply := func(text string) IdentityDecision {
		return IdentityDecision{Stop: true, Reply: channel.Message{Text: text}}
	}
	if !code.UsedAt.IsZero() {
		return true, reply("Bind code already used."), "", nil
	}
	if !code.ExpiresAt.IsZero() && time.Now().UTC().After(code.ExpiresAt) {
		return true, reply("Bind code expired."), "", nil
	}
	if strings.TrimSpace(code.Platform) != "" && !strings.EqualFold(strings.TrimSpace(code.Platform), msg.Channel.String()) {
		return true, reply("Bind code mismatch."), "", nil
	}
	if subjectID == "" {
		return true, reply("Cannot identify current account."), "", nil
	}

	// Consume: mark used + link source channel identity to issuer user.
	if err := r.bind.Consume(ctx, code, channelIdentityID); err != nil {
		switch {
		case errors.Is(err, bind.ErrCodeUsed):
			return true, reply("Bind code already used."), "", nil
		case errors.Is(err, bind.ErrCodeExpired):
			return true, reply("Bind code expired."), "", nil
		case errors.Is(err, bind.ErrCodeMismatch):
			return true, reply("Bind code mismatch."), "", nil
		case errors.Is(err, bind.ErrLinkConflict):
			return true, reply("Current identity has already been linked to another account."), "", nil
		default:
			return true, IdentityDecision{}, "", fmt.Errorf("consume bind code: %w", err)
		}
	}

	// Resolve linked user after binding.
	newUserID := code.IssuedByUserID
	if r.channelIdentities != nil {
		linkedUserID, err := r.channelIdentities.GetLinkedUserID(ctx, channelIdentityID)
		if err != nil {
			return true, IdentityDecision{}, "", fmt.Errorf("resolve linked user after bind: %w", err)
		}
		if strings.TrimSpace(linkedUserID) != "" {
			newUserID = linkedUserID
		}
	}

	return true, reply(r.bindReply), newUserID, nil
}

func extractSubjectIdentity(msg channel.InboundMessage) string {
	if strings.TrimSpace(msg.Sender.SubjectID) != "" {
		return strings.TrimSpace(msg.Sender.SubjectID)
	}
	if value := strings.TrimSpace(msg.Sender.Attribute("open_id")); value != "" {
		return value
	}
	if value := strings.TrimSpace(msg.Sender.Attribute("user_id")); value != "" {
		return value
	}
	if value := strings.TrimSpace(msg.Sender.Attribute("username")); value != "" {
		return value
	}
	return strings.TrimSpace(msg.Sender.DisplayName)
}

func identitySubjectCandidates(msg channel.InboundMessage, primary string) []string {
	candidates := make([]string, 0, 3)
	appendUnique := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if slices.Contains(candidates, value) {
			return
		}
		candidates = append(candidates, value)
	}

	appendUnique(primary)
	appendUnique(msg.Sender.Attribute("open_id"))
	appendUnique(msg.Sender.Attribute("user_id"))
	return candidates
}

func extractDisplayName(msg channel.InboundMessage) string {
	if strings.TrimSpace(msg.Sender.DisplayName) != "" {
		return strings.TrimSpace(msg.Sender.DisplayName)
	}
	if value := strings.TrimSpace(msg.Sender.Attribute("display_name")); value != "" {
		return value
	}
	if value := strings.TrimSpace(msg.Sender.Attribute("name")); value != "" {
		return value
	}
	if value := strings.TrimSpace(msg.Sender.Attribute("username")); value != "" {
		return value
	}
	return ""
}

// resolveProfile resolves display name and avatar URL for the sender.
// Always queries directory for avatar; prefers message-level display name over directory name.
func (r *IdentityResolver) resolveProfile(ctx context.Context, cfg channel.Config, msg channel.InboundMessage, subjectID string) (string, string) {
	displayName := extractDisplayName(msg)
	dirName, avatarURL := r.resolveProfileFromDirectory(ctx, cfg, msg, subjectID)
	if displayName == "" {
		displayName = dirName
	}
	return displayName, avatarURL
}

// resolveProfileFromDirectory looks up the directory for sender display name and avatar URL.
func (r *IdentityResolver) resolveProfileFromDirectory(ctx context.Context, cfg channel.Config, msg channel.InboundMessage, subjectID string) (string, string) {
	if r.registry == nil {
		return "", ""
	}
	subjectID = strings.TrimSpace(subjectID)
	if subjectID == "" {
		return "", ""
	}
	directoryAdapter, ok := r.registry.DirectoryAdapter(msg.Channel)
	if !ok || directoryAdapter == nil {
		return "", ""
	}
	if ctx == nil {
		ctx = context.Background()
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	entry, err := directoryAdapter.ResolveEntry(lookupCtx, cfg, subjectID, channel.DirectoryEntryUser)
	if err != nil {
		if r.logger != nil {
			r.logger.Debug(
				"resolve profile from directory failed",
				slog.String("channel", msg.Channel.String()),
				slog.String("subject_id", subjectID),
				slog.Any("error", err),
			)
		}
		return "", ""
	}
	name := strings.TrimSpace(entry.Name)
	if name == "" {
		name = strings.TrimSpace(entry.Handle)
	}
	return name, strings.TrimSpace(entry.AvatarURL)
}

func extractThreadID(msg channel.InboundMessage) string {
	if msg.Message.Thread != nil && strings.TrimSpace(msg.Message.Thread.ID) != "" {
		return strings.TrimSpace(msg.Message.Thread.ID)
	}
	if strings.TrimSpace(msg.Conversation.ThreadID) != "" {
		return strings.TrimSpace(msg.Conversation.ThreadID)
	}
	return ""
}

func isGroupConversationType(conversationType string) bool {
	ct := strings.ToLower(strings.TrimSpace(conversationType))
	if ct == "" {
		return false
	}
	return ct != "p2p" && ct != "private" && ct != "direct"
}

func (r *IdentityResolver) tryLinkConfiglessChannelIdentityToUser(ctx context.Context, msg channel.InboundMessage, channelIdentityID string) string {
	if r.registry == nil || !r.registry.IsConfigless(msg.Channel) {
		return ""
	}
	if r.channelIdentities == nil {
		return ""
	}
	candidateUserID := strings.TrimSpace(msg.Sender.Attribute("user_id"))
	if candidateUserID == "" {
		return ""
	}
	if err := r.channelIdentities.LinkChannelIdentityToUser(ctx, channelIdentityID, candidateUserID); err != nil {
		if r.logger != nil {
			r.logger.Warn("auto link configless channel identity failed",
				slog.String("channel", msg.Channel.String()),
				slog.String("channel_identity_id", channelIdentityID),
				slog.String("user_id", candidateUserID),
				slog.Any("error", err),
			)
		}
		return ""
	}
	return candidateUserID
}
