package directory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/contacts"
)

var (
	ErrNotFound    = errors.New("directory entry not found")
	ErrAmbiguous   = errors.New("directory entry ambiguous")
	ErrUnsupported = errors.New("directory operation unsupported")
)

type ContactReader interface {
	Search(ctx context.Context, botID, query string) ([]contacts.Contact, error)
	ListByBot(ctx context.Context, botID string) ([]contacts.Contact, error)
	ListChannelsByContact(ctx context.Context, contactID string) ([]contacts.ContactChannel, error)
}

type ChannelSessionStore interface {
	ListSessionsByBotPlatform(ctx context.Context, botID, platform string) ([]channel.ChannelSession, error)
}

type LocalService struct {
	contacts ContactReader
	sessions ChannelSessionStore
	logger   *slog.Logger
}

func NewLocalService(log *slog.Logger, contacts ContactReader, sessions ChannelSessionStore) *LocalService {
	if log == nil {
		log = slog.Default()
	}
	return &LocalService{
		contacts: contacts,
		sessions: sessions,
		logger:   log.With(slog.String("service", "directory")),
	}
}

func (s *LocalService) ListPeers(ctx context.Context, botID, platform, query string, limit int) ([]channel.DirectoryEntry, error) {
	if s.contacts == nil {
		return nil, fmt.Errorf("contacts service not configured")
	}
	trimmed := strings.TrimSpace(query)
	var items []contacts.Contact
	var err error
	if trimmed == "" {
		items, err = s.contacts.ListByBot(ctx, botID)
	} else {
		items, err = s.contacts.Search(ctx, botID, trimmed)
	}
	if err != nil {
		return nil, err
	}
	results := make([]channel.DirectoryEntry, 0, len(items))
	for _, contact := range items {
		channels, err := s.contacts.ListChannelsByContact(ctx, contact.ID)
		if err != nil {
			if s.logger != nil {
				s.logger.Warn("list contact channels failed", slog.String("contact_id", contact.ID), slog.Any("error", err))
			}
			continue
		}
		for _, ch := range channels {
			if platform != "" && ch.Platform != platform {
				continue
			}
			entry := channel.DirectoryEntry{
				Kind:     channel.DirectoryEntryUser,
				ID:       strings.TrimSpace(ch.ExternalID),
				Name:     chooseContactName(contact, ch),
				Handle:   strings.TrimSpace(contact.Alias),
				Metadata: map[string]any{},
			}
			if entry.ID == "" {
				continue
			}
			entry.Metadata["contact_id"] = contact.ID
			if contact.UserID != "" {
				entry.Metadata["user_id"] = contact.UserID
			}
			entry.Metadata["platform"] = ch.Platform
			results = append(results, entry)
			if limit > 0 && len(results) >= limit {
				return results, nil
			}
		}
	}
	return results, nil
}

func (s *LocalService) ListGroups(ctx context.Context, botID, platform, query string, limit int) ([]channel.DirectoryEntry, error) {
	if s.sessions == nil {
		return nil, fmt.Errorf("channel session store not configured")
	}
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return nil, fmt.Errorf("platform is required")
	}
	sessions, err := s.sessions.ListSessionsByBotPlatform(ctx, botID, platform)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(query)
	results := make([]channel.DirectoryEntry, 0, len(sessions))
	for _, session := range sessions {
		if !isGroupSession(session) {
			continue
		}
		name := channel.ReadString(session.Metadata, "conversation_name", "name")
		entryID := strings.TrimSpace(session.ReplyTarget)
		if entryID == "" {
			entryID = strings.TrimSpace(session.SessionID)
		}
		if entryID == "" {
			continue
		}
		if trimmed != "" && !matchesQuery(trimmed, entryID, name) {
			continue
		}
		results = append(results, channel.DirectoryEntry{
			Kind:     channel.DirectoryEntryGroup,
			ID:       entryID,
			Name:     strings.TrimSpace(name),
			Metadata: session.Metadata,
		})
		if limit > 0 && len(results) >= limit {
			return results, nil
		}
	}
	return results, nil
}

func (s *LocalService) ListGroupMembers(ctx context.Context, botID, platform, groupID string, limit int) ([]channel.DirectoryEntry, error) {
	return nil, ErrUnsupported
}

func (s *LocalService) ResolveTarget(ctx context.Context, botID, platform, input string, kind channel.DirectoryEntryKind) (channel.DirectoryEntry, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return channel.DirectoryEntry{}, ErrNotFound
	}
	switch kind {
	case channel.DirectoryEntryGroup:
		items, err := s.ListGroups(ctx, botID, platform, trimmed, 5)
		if err != nil {
			return channel.DirectoryEntry{}, err
		}
		return pickSingleMatch(items, trimmed)
	default:
		items, err := s.ListPeers(ctx, botID, platform, trimmed, 5)
		if err != nil {
			return channel.DirectoryEntry{}, err
		}
		return pickSingleMatch(items, trimmed)
	}
}

func pickSingleMatch(items []channel.DirectoryEntry, input string) (channel.DirectoryEntry, error) {
	if len(items) == 0 {
		return channel.DirectoryEntry{}, ErrNotFound
	}
	if len(items) == 1 {
		return items[0], nil
	}
	lower := strings.ToLower(strings.TrimSpace(input))
	var exact *channel.DirectoryEntry
	for i := range items {
		if strings.ToLower(strings.TrimSpace(items[i].ID)) == lower {
			exact = &items[i]
			break
		}
		if strings.ToLower(strings.TrimSpace(items[i].Name)) == lower {
			exact = &items[i]
			break
		}
	}
	if exact != nil {
		return *exact, nil
	}
	return channel.DirectoryEntry{}, ErrAmbiguous
}

func chooseContactName(contact contacts.Contact, ch contacts.ContactChannel) string {
	if strings.TrimSpace(contact.DisplayName) != "" {
		return strings.TrimSpace(contact.DisplayName)
	}
	if strings.TrimSpace(contact.Alias) != "" {
		return strings.TrimSpace(contact.Alias)
	}
	if strings.TrimSpace(ch.ExternalID) != "" {
		return strings.TrimSpace(ch.ExternalID)
	}
	return ""
}

func isGroupSession(session channel.ChannelSession) bool {
	value := strings.ToLower(strings.TrimSpace(channel.ReadString(session.Metadata, "conversation_type", "chat_type", "type")))
	if value == "" {
		return false
	}
	if strings.Contains(value, "group") {
		return true
	}
	return false
}

func matchesQuery(query string, fields ...string) bool {
	needle := strings.ToLower(strings.TrimSpace(query))
	if needle == "" {
		return true
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(strings.TrimSpace(field)), needle) {
			return true
		}
	}
	return false
}
