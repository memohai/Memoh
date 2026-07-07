package flow

import (
	"context"
	"errors"

	"github.com/memohai/memoh/internal/session"
)

type fakeBackgroundSessionService struct {
	getFn            func(ctx context.Context, sessionID string) (session.Session, error)
	updateMetadataFn func(ctx context.Context, sessionID string, metadata map[string]any) (session.Session, error)
}

func (f *fakeBackgroundSessionService) Get(ctx context.Context, sessionID string) (session.Session, error) {
	if f == nil || f.getFn == nil {
		return session.Session{}, errors.New("unexpected Get call")
	}
	return f.getFn(ctx, sessionID)
}

func (*fakeBackgroundSessionService) UpdateTitle(context.Context, string, string) (session.Session, error) {
	return session.Session{}, errors.New("unexpected UpdateTitle call")
}

func (f *fakeBackgroundSessionService) UpdateMetadata(ctx context.Context, sessionID string, metadata map[string]any) (session.Session, error) {
	if f == nil || f.updateMetadataFn == nil {
		return session.Session{}, errors.New("unexpected UpdateMetadata call")
	}
	return f.updateMetadataFn(ctx, sessionID, metadata)
}
