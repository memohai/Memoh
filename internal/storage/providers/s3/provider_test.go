package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/memohai/memoh/internal/storage"
)

type fakeClient struct {
	objects map[string][]byte
}

func newFakeClient() *fakeClient {
	return &fakeClient{objects: make(map[string][]byte)}
}

func (f *fakeClient) PutObject(_ context.Context, input *awss3.PutObjectInput, _ ...func(*awss3.Options)) (*awss3.PutObjectOutput, error) {
	data, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	f.objects[aws.ToString(input.Key)] = data
	return &awss3.PutObjectOutput{}, nil
}

func (f *fakeClient) GetObject(_ context.Context, input *awss3.GetObjectInput, _ ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	data, ok := f.objects[aws.ToString(input.Key)]
	if !ok {
		return nil, &types.NoSuchKey{}
	}
	return &awss3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(data))}, nil
}

func (f *fakeClient) DeleteObject(_ context.Context, input *awss3.DeleteObjectInput, _ ...func(*awss3.Options)) (*awss3.DeleteObjectOutput, error) {
	delete(f.objects, aws.ToString(input.Key))
	return &awss3.DeleteObjectOutput{}, nil
}

func (f *fakeClient) ListObjectsV2(_ context.Context, input *awss3.ListObjectsV2Input, _ ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error) {
	prefix := aws.ToString(input.Prefix)
	keys := make([]string, 0)
	for key := range f.objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	contents := make([]types.Object, 0, len(keys))
	for _, key := range keys {
		contents = append(contents, types.Object{Key: aws.String(key)})
	}
	return &awss3.ListObjectsV2Output{Contents: contents}, nil
}

func TestProviderCRUDAndPrefix(t *testing.T) {
	t.Parallel()

	client := newFakeClient()
	provider, err := New(client, "media", "memoh/assets")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx := context.Background()
	const key = "bot-1/ab/abcdef.png"
	if err := provider.Put(ctx, key, strings.NewReader("png")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if got := string(client.objects["memoh/assets/"+key]); got != "png" {
		t.Fatalf("stored bytes = %q, want png", got)
	}

	reader, err := provider.Open(ctx, key)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	data, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil || string(data) != "png" {
		t.Fatalf("Open() data = %q, err = %v", data, err)
	}

	keys, err := provider.ListPrefix(ctx, "bot-1/ab/abc")
	if err != nil {
		t.Fatalf("ListPrefix() error = %v", err)
	}
	if len(keys) != 1 || keys[0] != key {
		t.Fatalf("ListPrefix() = %#v, want [%q]", keys, key)
	}
	if got := provider.AccessPath(ctx, key); got != "" {
		t.Fatalf("AccessPath() = %q, want empty private reference", got)
	}

	if err := provider.Delete(ctx, key); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := provider.Open(ctx, key); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("Open() after Delete() error = %v, want storage.ErrNotFound", err)
	}
}

func TestProviderRejectsInvalidConfigurationAndKeys(t *testing.T) {
	t.Parallel()

	if _, err := New(nil, "bucket", ""); err == nil {
		t.Fatal("New() with nil client unexpectedly succeeded")
	}
	if _, err := New(newFakeClient(), "", ""); err == nil {
		t.Fatal("New() with empty bucket unexpectedly succeeded")
	}
	if _, err := New(newFakeClient(), "bucket", "../escape"); err == nil {
		t.Fatal("New() with invalid prefix unexpectedly succeeded")
	}

	provider, err := New(newFakeClient(), "bucket", "")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	for _, key := range []string{"", "../escape", "a/../escape", "a//escape", "/absolute", `windows\\path`} {
		if err := provider.Put(context.Background(), key, strings.NewReader("x")); err == nil {
			t.Errorf("Put(%q) unexpectedly succeeded", key)
		}
	}
}
