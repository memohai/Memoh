package cutover

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/memohai/memoh/internal/storage"
)

type memoryProvider struct {
	objects map[string][]byte
	openErr error
	puts    int
}

func (p *memoryProvider) Put(_ context.Context, key string, reader io.Reader) error {
	p.puts++
	data, err := io.ReadAll(reader)
	if err == nil {
		p.objects[key] = data
	}
	return err
}

func (p *memoryProvider) Open(_ context.Context, key string) (io.ReadCloser, error) {
	if p.openErr != nil {
		return nil, p.openErr
	}
	data, ok := p.objects[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (p *memoryProvider) Delete(_ context.Context, key string) error {
	delete(p.objects, key)
	return nil
}

func (*memoryProvider) AccessPath(context.Context, string) string { return "" }

func TestProviderUsesLegacyOnlyForPrimaryMisses(t *testing.T) {
	t.Parallel()

	primary := &memoryProvider{objects: map[string][]byte{}}
	legacy := &memoryProvider{objects: map[string][]byte{"old": []byte("legacy")}}
	provider, err := New(primary, legacy)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	rc, err := provider.Open(context.Background(), "old")
	if err != nil {
		t.Fatalf("Open() legacy error = %v", err)
	}
	data, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(data) != "legacy" {
		t.Fatalf("Open() data = %q", data)
	}

	primary.openErr = errors.New("s3 unavailable")
	if _, err := provider.Open(context.Background(), "old"); err == nil || !errors.Is(err, primary.openErr) {
		t.Fatalf("Open() outage error = %v", err)
	}

	if err := provider.Put(context.Background(), "new", bytes.NewBufferString("canonical")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if primary.puts != 1 || legacy.puts != 0 {
		t.Fatalf("Put() counts primary=%d legacy=%d", primary.puts, legacy.puts)
	}
}
