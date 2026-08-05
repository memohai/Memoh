//go:build integration

package s3_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/config"
	storagefactory "github.com/memohai/memoh/internal/storage/factory"
)

func TestProviderAgainstS3CompatibleService(t *testing.T) {
	if strings.TrimSpace(os.Getenv("MEMOH_S3_INTEGRATION")) != "1" {
		t.Skip("set MEMOH_S3_INTEGRATION=1 to run the S3-compatible storage smoke test")
	}

	endpoint := envOrDefault(
		"MEMOH_S3_TEST_ENDPOINT",
		"http://127.0.0.1:"+envOrDefault("MEMOH_DEV_S3_PORT", "18333"),
	)
	accessKey := envOrDefault("MEMOH_DEV_S3_ACCESS_KEY_ID", "memoh")
	secretKey := envOrDefault("MEMOH_DEV_S3_SECRET_ACCESS_KEY", "memoh-development")
	bucket := envOrDefault("MEMOH_DEV_S3_BUCKET", "memoh-media")

	provider, err := storagefactory.NewS3(config.S3StorageConfig{
		Endpoint:        endpoint,
		Bucket:          bucket,
		AccessKeyID:     accessKey,
		SecretAccessKey: secretKey,
		Region:          "us-east-1",
		Prefix:          "memoh-integration",
		PathStyle:       true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	key := fmt.Sprintf("smoke/%d-%d.txt", os.Getpid(), time.Now().UnixNano())
	cleanup := true
	defer func() {
		if cleanup {
			_ = provider.Delete(context.Background(), key)
		}
	}()

	if err := provider.Put(ctx, key, strings.NewReader("memoh-s3-smoke")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	reader, err := provider.Open(ctx, key)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	payload, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil || string(payload) != "memoh-s3-smoke" {
		t.Fatalf("Open() payload = %q, err = %v", payload, readErr)
	}

	keys, err := provider.ListPrefix(ctx, "smoke")
	if err != nil {
		t.Fatalf("ListPrefix() error = %v", err)
	}
	found := false
	for _, listedKey := range keys {
		if listedKey == key {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ListPrefix() did not return %q: %#v", key, keys)
	}

	if err := provider.Delete(ctx, key); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	cleanup = false
	if _, err := provider.Open(ctx, key); err == nil {
		t.Fatal("Open() after Delete() unexpectedly succeeded")
	}
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
