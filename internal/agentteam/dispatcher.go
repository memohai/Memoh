package agentteam

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// Dispatcher is the agentteam-side wrapper that:
//   - parses mentions out of a freshly posted comment,
//   - creates handoff rows for each `bot` mention,
//   - schedules execution of the target bot's session.
//
// The execution callback is provided by the conversation layer to avoid an
// import cycle. When no Trigger function is configured the dispatcher only
// creates the handoff rows; callers are expected to drain pending handoffs
// at startup or via a periodic sweeper.
type Dispatcher struct {
	service *Service
	trigger TriggerFunc
	logger  *slog.Logger
}

// TriggerFunc executes a handoff for a target bot. Implementations should
// run an in-process agent turn for the bot, inject the issue / handoff
// context, and post the result back to the issue via a comment.
//
// The dispatcher passes a snapshot of the handoff and the originating
// comment so the runner can build a prompt without re-reading the database.
type TriggerFunc func(ctx context.Context, handoff Handoff, trigger Comment) error

// NewDispatcher builds a dispatcher around a Service.
func NewDispatcher(log *slog.Logger, service *Service) *Dispatcher {
	if log == nil {
		log = slog.Default()
	}
	return &Dispatcher{
		service: service,
		logger:  log.With(slog.String("service", "agentteam_dispatcher")),
	}
}

// SetTrigger registers the function used to dispatch a handoff to a bot.
func (d *Dispatcher) SetTrigger(fn TriggerFunc) {
	if d == nil {
		return
	}
	d.trigger = fn
}

// HandleComment is the central post-comment hook. It:
//  1. Parses bot/squad mentions from `comment.Content`.
//  2. Skips self-mentions and duplicate pending handoffs (anti-loop).
//  3. Creates a handoff per addressed bot, then triggers it.
//  4. When the comment author is a bot and an active handoff exists for the
//     author on this issue, the handoff is marked completed with the comment
//     as the result, and a return handoff is queued for the original delegator.
func (d *Dispatcher) HandleComment(ctx context.Context, comment Comment) error {
	if d == nil || d.service == nil {
		return errors.New("agentteam: dispatcher not configured")
	}
	if strings.TrimSpace(comment.ID) == "" {
		return errors.New("agentteam: comment ID required")
	}

	// Mentions in this comment that point at bots whose follow-up will
	// already be delivered through a return handoff — those are skipped
	// so the same bot does not wake up twice on the same reply.
	//
	// Important: we ONLY suppress when an active bot-from handoff was
	// just closed by `finalizeAndReturn`. In that case the from-bot is
	// going to be woken via the system return; @-ing them again here
	// would create a duplicate.
	//
	// Earlier versions of this branch additionally suppressed any bot
	// referenced by `ParentCommentID`'s author. That was too aggressive:
	// it also blocked legitimate "continue the debate" replies (bot A
	// answers bot B's question, then B writes `@A I disagree because…`).
	// In that scenario A has no active handoff left — the previous
	// return chain already terminated — and the new mention should
	// create a fresh handoff so A can actually respond. We rely on (A)
	// alone, which checks live handoff state instead of guessing from
	// the comment thread shape.
	skipMentionTargets := map[string]struct{}{}
	if comment.AuthorType == ActorBot && strings.TrimSpace(comment.AuthorBotID) != "" {
		completed, err := d.finalizeAndReturn(ctx, comment)
		if err != nil {
			d.logger.Warn("finalize handoff failed", slog.String("issue_id", comment.IssueID), slog.Any("error", err))
		}
		for _, ho := range completed {
			if ho.FromActorType == ActorBot && strings.TrimSpace(ho.FromBotID) != "" {
				skipMentionTargets[ho.FromBotID] = struct{}{}
			}
		}
	}

	d.dispatchMentionsInComment(ctx, comment, skipMentionTargets)
	return nil
}

// HandleIssueCreated is the post-CreateIssue hook. Mentions inside the
// initial description are treated like mentions in a top-level comment
// authored by whoever created the issue: a handoff is fanned out to each
// addressed bot, no comment row is involved (the description itself is
// the "trigger text"). The synthetic Comment carries an empty ID so the
// store records the resulting handoffs with a NULL trigger_comment_id;
// downstream finalize / return routing still works because the bot's
// reply will be a top-level comment which closes ALL pending handoffs
// for (bot, issue).
//
// `sourceSessionID` should be the session that authored the issue when
// known (set by the issue_create tool); HTTP-created issues pass "".
func (d *Dispatcher) HandleIssueCreated(ctx context.Context, issue Issue, sourceSessionID string) error {
	if d == nil || d.service == nil {
		return errors.New("agentteam: dispatcher not configured")
	}
	if strings.TrimSpace(issue.ID) == "" || strings.TrimSpace(issue.TeamID) == "" {
		return errors.New("agentteam: issue ID and team ID required")
	}
	if strings.TrimSpace(issue.Description) == "" {
		return nil
	}
	synthetic := Comment{
		IssueID:         issue.ID,
		TeamID:          issue.TeamID,
		AuthorType:      issue.CreatedByType,
		AuthorBotID:     issue.CreatedByBotID,
		AuthorUserID:    issue.CreatedByUserID,
		Content:         issue.Description,
		SourceSessionID: strings.TrimSpace(sourceSessionID),
	}
	if synthetic.AuthorType == "" {
		synthetic.AuthorType = ActorUser
	}
	d.dispatchMentionsInComment(ctx, synthetic, nil)
	return nil
}

// dispatchMentionsInComment is the shared mention → handoff fan-out used
// by both HandleComment (real posted comment) and HandleIssueCreated
// (synthetic comment derived from issue.description). skip is consulted
// to suppress duplicate wake-ups for bots that are already going to be
// woken via a queued system return.
func (d *Dispatcher) dispatchMentionsInComment(ctx context.Context, comment Comment, skip map[string]struct{}) {
	mentions := ParseMentions(comment.Content)
	if len(mentions) == 0 || strings.TrimSpace(comment.TeamID) == "" {
		return
	}
	roster, err := d.service.Store().ListMembers(ctx, comment.TeamID)
	if err != nil {
		d.logger.Warn(
			"list team members for mention resolution failed",
			slog.String("team_id", comment.TeamID),
			slog.Any("error", err),
		)
		return
	}
	for _, m := range mentions {
		member, ok := MatchMember(m.Name, roster)
		if !ok {
			// Unknown or ambiguous label; leave the @ as plain text so
			// the human reader still sees it without surprise routing.
			continue
		}
		if member.MemberType != MemberBot {
			// User mentions are notification-only — never spawn a handoff.
			continue
		}
		targetBotID := member.BotID
		if targetBotID == "" {
			continue
		}
		if comment.AuthorType == ActorBot && comment.AuthorBotID == targetBotID {
			continue
		}
		if _, ok := skip[targetBotID]; ok {
			continue
		}
		if _, err := d.createHandoffForMention(ctx, comment, targetBotID); err != nil {
			d.logger.Warn(
				"create handoff failed",
				slog.String("issue_id", comment.IssueID),
				slog.String("to_bot_id", targetBotID),
				slog.Any("error", err),
			)
			continue
		}
	}
}

func (d *Dispatcher) createHandoffForMention(ctx context.Context, comment Comment, targetBotID string) (Handoff, error) {
	existing, err := d.service.Store().ListPendingHandoffsToBotForIssue(ctx, targetBotID, comment.IssueID)
	if err != nil {
		return Handoff{}, fmt.Errorf("list pending handoffs: %w", err)
	}
	// Dedup is per (target_bot, issue, source_session). Two mentions from
	// the *same* session asking the same bot to do something are coalesced
	// (a single wake-up is enough). Mentions from different sessions are
	// independent — they each get their own handoff so the return path
	// can later route the result back to its specific origin.
	commentSrc := strings.TrimSpace(comment.SourceSessionID)
	for _, h := range existing {
		if strings.TrimSpace(h.SourceSessionID) == commentSrc {
			return h, nil
		}
	}
	actorType := comment.AuthorType
	if actorType == "" {
		actorType = ActorUser
	}
	// SourceSessionID is the from-side session (the session that *wrote*
	// the mention comment). For bot authors the value comes from the
	// comment's metadata, which the tool layer fills in when the bot
	// calls `issue_comment`. For human / system authors it stays empty
	// and the dispatcher's later finalize step simply skips queuing a
	// return back to a session that does not exist.
	handoff, err := d.service.Store().CreateHandoff(ctx, CreateHandoffInput{
		TeamID:           comment.TeamID,
		IssueID:          comment.IssueID,
		FromActorType:    actorType,
		FromBotID:        comment.AuthorBotID,
		FromUserID:       comment.AuthorUserID,
		ToBotID:          targetBotID,
		TriggerCommentID: comment.ID,
		SourceSessionID:  comment.SourceSessionID,
		Status:           HandoffPending,
	})
	if err != nil {
		return Handoff{}, fmt.Errorf("create handoff: %w", err)
	}
	if d.trigger != nil {
		go d.runHandoff(context.Background(), handoff, comment) //nolint:contextcheck // background job, intentionally detached from request context
	}
	return handoff, nil
}

func (d *Dispatcher) runHandoff(ctx context.Context, handoff Handoff, comment Comment) {
	defer func() {
		if r := recover(); r != nil {
			d.logger.Error(
				"handoff trigger panic",
				slog.String("handoff_id", handoff.ID),
				slog.Any("panic", r),
			)
		}
	}()
	if d.trigger == nil {
		return
	}
	if err := d.trigger(ctx, handoff, comment); err != nil {
		d.logger.Warn(
			"handoff trigger failed",
			slog.String("handoff_id", handoff.ID),
			slog.String("to_bot_id", handoff.ToBotID),
			slog.Any("error", err),
		)
		if _, ferr := d.service.Store().FailHandoff(ctx, handoff.ID, err.Error()); ferr != nil {
			d.logger.Warn(
				"mark handoff failed errored",
				slog.String("handoff_id", handoff.ID),
				slog.Any("error", ferr),
			)
		}
	}
}

// finalizeAndReturn closes active handoffs that the bot's comment
// addresses, and queues a single return handoff per closed delegation
// so the original delegator wakes up in their original session.
//
// Match precedence (per the design that "S1 的归 S1, S2 的归 S2, 即使
// 同一个 issue"):
//
//   - When the comment carries a `parent_comment_id`, the dispatcher
//     finds the one pending handoff whose `trigger_comment_id` matches
//     and closes only that handoff. This is the strict, threaded path
//     and is how the team tool encourages bots to reply.
//   - When the comment is top-level (no `parent_comment_id`), the
//     dispatcher falls back to "close every pending handoff for this
//     (bot, issue) pair" — the legacy behaviour. A top-level result
//     comment is treated as "I'm done with everything you asked me on
//     this issue".
//
// For each closed handoff, the return handoff inherits the original's
// `SourceSessionID` as its own `TargetSessionID` — pinning the
// delegator's wake-up back to the exact session that asked the
// question. When the original delegator was a human / system, no
// return is queued.
func (d *Dispatcher) finalizeAndReturn(ctx context.Context, comment Comment) ([]Handoff, error) {
	active, err := d.service.Store().ListPendingHandoffsToBotForIssue(ctx, comment.AuthorBotID, comment.IssueID)
	if err != nil {
		return nil, err
	}

	toClose := matchHandoffsForReply(active, comment)
	completedHandoffs := make([]Handoff, 0, len(toClose))
	for _, ho := range toClose {
		completed, err := d.service.Store().CompleteHandoff(ctx, ho.ID, comment.ID)
		if err != nil {
			d.logger.Warn("complete handoff failed", slog.String("handoff_id", ho.ID), slog.Any("error", err))
			continue
		}
		completedHandoffs = append(completedHandoffs, completed)
		if completed.FromActorType == ActorBot && strings.TrimSpace(completed.FromBotID) != "" {
			d.queueReturn(ctx, completed, comment)
		}
	}
	return completedHandoffs, nil
}

// matchHandoffsForReply implements the per-mention closure policy. See
// finalizeAndReturn for the full semantics.
func matchHandoffsForReply(active []Handoff, comment Comment) []Handoff {
	parent := strings.TrimSpace(comment.ParentCommentID)
	if parent == "" {
		// Top-level comment — close everything pending for this bot on
		// this issue.
		return active
	}
	for _, ho := range active {
		if strings.TrimSpace(ho.TriggerCommentID) == parent {
			return []Handoff{ho}
		}
	}
	// Threaded reply but the parent does not correspond to any pending
	// trigger comment — leave handoffs untouched so the next on-trigger
	// reply can close them properly.
	return nil
}

func (d *Dispatcher) queueReturn(ctx context.Context, original Handoff, resultComment Comment) {
	// Anti-loop: skip when an equivalent return is already pending. The
	// scope is intentionally narrow — same (delegator bot, issue,
	// originating session). Returns destined for *different* originating
	// sessions on the same issue must coexist (S1's return must not
	// block S2's), which is the whole point of the per-mention routing
	// design.
	pending, err := d.service.Store().ListPendingHandoffsToBotForIssue(ctx, original.FromBotID, original.IssueID)
	if err != nil {
		d.logger.Warn("list pending returns failed", slog.String("handoff_id", original.ID), slog.Any("error", err))
		return
	}
	for _, p := range pending {
		// Only a system-generated return that aims at the same
		// originating session counts as a duplicate.
		if p.FromActorType == ActorSystem && strings.TrimSpace(p.TargetSessionID) == strings.TrimSpace(original.SourceSessionID) {
			return
		}
	}
	returnMeta := []byte(fmt.Sprintf(`{"return_for_handoff":"%s"}`, original.ID))
	// TargetSessionID = original.SourceSessionID is the whole point of
	// this design: when A in session S1 delegated to B, the return for
	// A must wake A back up in S1, not in some per-issue scratch
	// session. When the original handoff had no source session (e.g.
	// the user posted the mention via the web UI), the field stays
	// empty and the resolver falls back to a per-issue session.
	ret, err := d.service.Store().CreateHandoff(ctx, CreateHandoffInput{
		TeamID:           original.TeamID,
		IssueID:          original.IssueID,
		FromActorType:    ActorSystem,
		ToBotID:          original.FromBotID,
		TriggerCommentID: resultComment.ID,
		TargetSessionID:  original.SourceSessionID,
		Status:           HandoffPending,
		Metadata:         returnMeta,
	})
	if err != nil {
		d.logger.Warn("create return handoff failed", slog.String("handoff_id", original.ID), slog.Any("error", err))
		return
	}
	if _, err := d.service.Store().SetHandoffReturn(ctx, original.ID, ret.ID); err != nil {
		d.logger.Warn("set return handoff link failed", slog.String("handoff_id", original.ID), slog.Any("error", err))
	}
	if d.trigger != nil {
		go d.runHandoff(context.Background(), ret, resultComment) //nolint:contextcheck // background job, intentionally detached from request context
	}
}
