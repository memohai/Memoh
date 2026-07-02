package flow

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

type fakeRetryStore struct {
	assets []dbsqlc.ListMessageAssetsRow
}

func (*fakeRetryStore) GetVisibleAssistantTurnForRetry(context.Context, dbsqlc.GetVisibleAssistantTurnForRetryParams) (dbsqlc.GetVisibleAssistantTurnForRetryRow, error) {
	return dbsqlc.GetVisibleAssistantTurnForRetryRow{}, nil
}

func (s *fakeRetryStore) ListMessageAssets(context.Context, pgtype.UUID) ([]dbsqlc.ListMessageAssetsRow, error) {
	return s.assets, nil
}

func TestRewriteRequestAttachmentsRestoresPersistedAssetRefs(t *testing.T) {
	t.Parallel()

	attachments, err := rewriteRequestAttachments(context.Background(), &fakeRetryStore{
		assets: []dbsqlc.ListMessageAssetsRow{{
			ContentHash: "image-hash",
			Name:        "photo.png",
			Metadata:    []byte(`{"storage_key":"ab/image-hash.png"}`),
		}, {
			ContentHash: "file-hash",
			Metadata:    []byte(`{"name":"notes.pdf","storage_key":"cd/file-hash.pdf"}`),
		}},
	}, testUUID(1))
	if err != nil {
		t.Fatalf("rewriteRequestAttachments() error = %v", err)
	}
	if len(attachments) != 2 {
		t.Fatalf("attachments len = %d, want 2", len(attachments))
	}
	if got := attachments[0]; got.ContentHash != "image-hash" || got.Type != "image" || got.Path != "/data/media/ab/image-hash.png" || got.Name != "photo.png" {
		t.Fatalf("image attachment = %#v", got)
	}
	if got := attachments[1]; got.ContentHash != "file-hash" || got.Type != "file" || got.Path != "/data/media/cd/file-hash.pdf" || got.Name != "notes.pdf" {
		t.Fatalf("file attachment = %#v", got)
	}
}
