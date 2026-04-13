package channel_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
)

const dirTestChannelType = channel.ChannelType("dir-test")

// dirMockAdapter implements Adapter and ChannelDirectoryAdapter for registry DirectoryAdapter tests.
type dirMockAdapter struct{}

func (*dirMockAdapter) Type() channel.ChannelType { return dirTestChannelType }

func (*dirMockAdapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{Type: dirTestChannelType, DisplayName: "DirTest"}
}

func (*dirMockAdapter) ListPeers(_ context.Context, _ channel.ChannelConfig, _ channel.DirectoryQuery) ([]channel.DirectoryEntry, error) {
	return nil, nil
}

func (*dirMockAdapter) ListGroups(_ context.Context, _ channel.ChannelConfig, _ channel.DirectoryQuery) ([]channel.DirectoryEntry, error) {
	return nil, nil
}

func (*dirMockAdapter) ListGroupMembers(_ context.Context, _ channel.ChannelConfig, _ string, _ channel.DirectoryQuery) ([]channel.DirectoryEntry, error) {
	return nil, nil
}

func (*dirMockAdapter) ResolveEntry(_ context.Context, _ channel.ChannelConfig, _ string, _ channel.DirectoryEntryKind) (channel.DirectoryEntry, error) {
	return channel.DirectoryEntry{}, nil
}

func TestDirectoryAdapter_Unsupported(t *testing.T) {
	t.Parallel()
	reg := newTestConfigRegistry()
	dir, ok := reg.DirectoryAdapter(testChannelType)
	if ok || dir != nil {
		t.Fatalf("DirectoryAdapter(test) = (%v, %v), want (nil, false)", dir, ok)
	}
}

func TestDirectoryAdapter_Supported(t *testing.T) {
	t.Parallel()
	reg := channel.NewRegistry()
	reg.MustRegister(&dirMockAdapter{})
	dir, ok := reg.DirectoryAdapter(dirTestChannelType)
	if !ok || dir == nil {
		t.Fatalf("DirectoryAdapter(dir-test) = (%v, %v), want (non-nil, true)", dir, ok)
	}
}

func TestDirectoryAdapter_UnknownType(t *testing.T) {
	t.Parallel()
	reg := channel.NewRegistry()
	dir, ok := reg.DirectoryAdapter(channel.ChannelType("unknown"))
	if ok || dir != nil {
		t.Fatalf("DirectoryAdapter(unknown) = (%v, %v), want (nil, false)", dir, ok)
	}
}

type attachmentResolverAdapter struct{}

func (*attachmentResolverAdapter) Type() channel.ChannelType {
	return channel.ChannelType("attachment-test")
}

func (*attachmentResolverAdapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{Type: channel.ChannelType("attachment-test"), DisplayName: "AttachmentTest"}
}

func (*attachmentResolverAdapter) ResolveAttachment(_ context.Context, _ channel.ChannelConfig, _ channel.Attachment) (channel.AttachmentPayload, error) {
	return channel.AttachmentPayload{
		Reader: io.NopCloser(strings.NewReader("payload")),
		Mime:   "text/plain",
		Name:   "payload.txt",
		Size:   7,
	}, nil
}

type failingResolverAdapter struct{}

func (*failingResolverAdapter) Type() channel.ChannelType {
	return channel.ChannelType("failing-attachment-test")
}

func (*failingResolverAdapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{Type: channel.ChannelType("failing-attachment-test"), DisplayName: "FailingAttachmentTest"}
}

func (*failingResolverAdapter) ResolveAttachment(_ context.Context, _ channel.ChannelConfig, att channel.Attachment) (channel.AttachmentPayload, error) {
	if strings.TrimSpace(att.PlatformKey) == "" {
		return channel.AttachmentPayload{}, channel.ErrAttachmentNotResolvable
	}
	return channel.AttachmentPayload{}, errors.New("platform API error")
}

func TestGetAttachmentResolver_Supported(t *testing.T) {
	t.Parallel()
	reg := channel.NewRegistry()
	reg.MustRegister(&attachmentResolverAdapter{})
	resolver, ok := reg.GetAttachmentResolver(channel.ChannelType("attachment-test"))
	if !ok || resolver == nil {
		t.Fatalf("GetAttachmentResolver should return resolver for supported adapter")
	}
}

func TestGetAttachmentResolver_Unsupported(t *testing.T) {
	t.Parallel()
	reg := newTestConfigRegistry()
	resolver, ok := reg.GetAttachmentResolver(testChannelType)
	if ok || resolver != nil {
		t.Fatalf("GetAttachmentResolver(test) = (%v, %v), want (nil, false)", resolver, ok)
	}
}

func TestEffectiveResolver_PublicURL(t *testing.T) {
	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://public.example.test/cat.png" {
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("not found")), Header: make(http.Header)}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(strings.NewReader("png-bytes")),
		}, nil
	})
	defer func() { http.DefaultTransport = oldTransport }()

	reg := newTestConfigRegistry()
	resolver := reg.EffectiveAttachmentResolver(testChannelType)
	payload, err := resolver.ResolveAttachment(context.Background(), channel.ChannelConfig{}, channel.Attachment{
		Type: channel.AttachmentImage,
		URL:  "https://public.example.test/cat.png",
	})
	if err != nil {
		t.Fatalf("ResolveAttachment: %v", err)
	}
	defer func() { _ = payload.Reader.Close() }()
	data, err := io.ReadAll(payload.Reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "png-bytes" {
		t.Fatalf("unexpected payload: %q", string(data))
	}
}

func TestEffectiveResolver_URLWithPlatformKey(t *testing.T) {
	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://cdn.discordapp.com/attachments/file.png" {
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("not found")), Header: make(http.Header)}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(strings.NewReader("discord-bytes")),
		}, nil
	})
	defer func() { http.DefaultTransport = oldTransport }()

	reg := newTestConfigRegistry()
	resolver := reg.EffectiveAttachmentResolver(testChannelType)
	att := channel.Attachment{
		Type:        channel.AttachmentImage,
		URL:         "https://cdn.discordapp.com/attachments/file.png",
		PlatformKey: "discord-file-id",
	}
	payload, err := resolver.ResolveAttachment(context.Background(), channel.ChannelConfig{}, att)
	if err != nil {
		t.Fatalf("ResolveAttachment: %v", err)
	}
	defer func() { _ = payload.Reader.Close() }()
	data, err := io.ReadAll(payload.Reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "discord-bytes" {
		t.Fatalf("unexpected payload: %q", string(data))
	}
}

func TestEffectiveResolver_URLWithSourcePlatform(t *testing.T) {
	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://qq.example.test/files/image.jpg" {
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("not found")), Header: make(http.Header)}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/jpeg"}},
			Body:       io.NopCloser(strings.NewReader("qq-bytes")),
		}, nil
	})
	defer func() { http.DefaultTransport = oldTransport }()

	reg := newTestConfigRegistry()
	resolver := reg.EffectiveAttachmentResolver(testChannelType)
	att := channel.Attachment{
		Type:           channel.AttachmentImage,
		URL:            "https://qq.example.test/files/image.jpg",
		SourcePlatform: "qq",
	}
	payload, err := resolver.ResolveAttachment(context.Background(), channel.ChannelConfig{}, att)
	if err != nil {
		t.Fatalf("ResolveAttachment: %v", err)
	}
	defer func() { _ = payload.Reader.Close() }()
	data, err := io.ReadAll(payload.Reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "qq-bytes" {
		t.Fatalf("unexpected payload: %q", string(data))
	}
}

func TestEffectiveResolver_RejectsHTMLImage(t *testing.T) {
	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://public.example.test/login.html" {
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("not found")), Header: make(http.Header)}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/html"}},
			Body:       io.NopCloser(strings.NewReader("<html>login</html>")),
		}, nil
	})
	defer func() { http.DefaultTransport = oldTransport }()

	reg := newTestConfigRegistry()
	resolver := reg.EffectiveAttachmentResolver(testChannelType)
	_, err := resolver.ResolveAttachment(context.Background(), channel.ChannelConfig{}, channel.Attachment{
		Type: channel.AttachmentImage,
		URL:  "https://public.example.test/login.html",
	})
	if err == nil || !strings.Contains(err.Error(), "html") {
		t.Fatalf("expected html rejection error, got %v", err)
	}
}

func TestEffectiveResolver_URLDoesNotEagerlyBufferBody(t *testing.T) {
	body := &limitedResolveBody{
		data:       bytes.Repeat([]byte("z"), 2048),
		readBudget: 512,
		enforce:    true,
	}
	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://public.example.test/stream.bin" {
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("not found")), Header: make(http.Header)}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/octet-stream"}},
			Body:       body,
		}, nil
	})
	defer func() { http.DefaultTransport = oldTransport }()

	reg := newTestConfigRegistry()
	resolver := reg.EffectiveAttachmentResolver(testChannelType)
	payload, err := resolver.ResolveAttachment(context.Background(), channel.ChannelConfig{}, channel.Attachment{
		Type: channel.AttachmentFile,
		URL:  "https://public.example.test/stream.bin",
	})
	if err != nil {
		t.Fatalf("ResolveAttachment: %v", err)
	}
	defer func() { _ = payload.Reader.Close() }()

	body.enforce = false
	data, err := io.ReadAll(payload.Reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(data) != 2048 {
		t.Fatalf("expected 2048 bytes, got %d", len(data))
	}
}

func TestEffectiveResolver_PlatformError(t *testing.T) {
	reg := channel.NewRegistry()
	reg.MustRegister(&failingResolverAdapter{})
	resolver := reg.EffectiveAttachmentResolver(channel.ChannelType("failing-attachment-test"))

	att := channel.Attachment{
		Type:        channel.AttachmentFile,
		PlatformKey: "F123",
	}

	_, err := resolver.ResolveAttachment(context.Background(), channel.ChannelConfig{}, att)
	if err == nil {
		t.Fatal("expected error when platform fails and no fallback is viable")
	}
	if !strings.Contains(err.Error(), "platform API error") {
		t.Fatalf("expected platform error to propagate, got: %v", err)
	}
}

// Platform-owned attachments must not fall back to anonymous URL download.
func TestEffectiveResolver_PlatformOwnsURL(t *testing.T) {
	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		t.Fatal("default resolver must not be attempted when platform resolver owns the attachment")
		return nil, nil
	})
	defer func() { http.DefaultTransport = oldTransport }()

	reg := channel.NewRegistry()
	reg.MustRegister(&failingResolverAdapter{})
	resolver := reg.EffectiveAttachmentResolver(channel.ChannelType("failing-attachment-test"))

	att := channel.Attachment{
		Type:        channel.AttachmentFile,
		URL:         "https://files.slack.test/private/doc.pdf",
		PlatformKey: "F123",
		Name:        "doc.pdf",
		Mime:        "application/pdf",
	}

	_, err := resolver.ResolveAttachment(context.Background(), channel.ChannelConfig{}, att)
	if err == nil {
		t.Fatal("expected platform error to propagate, got nil")
	}
	if !strings.Contains(err.Error(), "platform API error") {
		t.Fatalf("expected platform error, got: %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type limitedResolveBody struct {
	data       []byte
	offset     int
	readBudget int
	enforce    bool
}

func (r *limitedResolveBody) Read(p []byte) (int, error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	if r.enforce && r.offset >= r.readBudget {
		return 0, errors.New("resolver eagerly buffered attachment body")
	}

	allowed := len(p)
	if r.enforce {
		remaining := r.readBudget - r.offset
		if remaining < allowed {
			allowed = remaining
		}
	}
	if remaining := len(r.data) - r.offset; remaining < allowed {
		allowed = remaining
	}
	copy(p, r.data[r.offset:r.offset+allowed])
	r.offset += allowed
	if r.offset >= len(r.data) {
		return allowed, io.EOF
	}
	return allowed, nil
}

func (*limitedResolveBody) Close() error {
	return nil
}
