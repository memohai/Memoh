// Package s3 implements storage.Provider for S3-compatible object stores.
package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	"github.com/memohai/memoh/internal/storage"
)

var (
	_ storage.Provider                  = (*Provider)(nil)
	_ storage.PrefixLister              = (*Provider)(nil)
	_ storage.AuthoritativePrefixLister = (*Provider)(nil)
)

// Client is the subset of the AWS S3 client used by Provider. Keeping the
// interface here makes S3-compatible implementations easy to test without a
// network service.
type Client interface {
	PutObject(context.Context, *awss3.PutObjectInput, ...func(*awss3.Options)) (*awss3.PutObjectOutput, error)
	GetObject(context.Context, *awss3.GetObjectInput, ...func(*awss3.Options)) (*awss3.GetObjectOutput, error)
	DeleteObject(context.Context, *awss3.DeleteObjectInput, ...func(*awss3.Options)) (*awss3.DeleteObjectOutput, error)
	ListObjectsV2(context.Context, *awss3.ListObjectsV2Input, ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error)
}

// Provider stores private media objects in an S3-compatible bucket. It never
// returns a public or pre-signed URL; public delivery remains an upper-layer
// concern so bucket privacy is not coupled to storage.
type Provider struct {
	client Client
	bucket string
	prefix string
}

// New creates an S3-backed storage provider. client may be an *s3.Client
// configured for AWS S3, MinIO, R2, or another S3-compatible endpoint.
func New(client Client, bucket, prefix string) (*Provider, error) {
	if client == nil {
		return nil, errors.New("s3 client is required")
	}
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return nil, errors.New("s3 bucket is required")
	}
	prefix, err := cleanPrefix(prefix)
	if err != nil {
		return nil, err
	}
	return &Provider{client: client, bucket: bucket, prefix: prefix}, nil
}

func (p *Provider) Put(ctx context.Context, key string, reader io.Reader) error {
	if reader == nil {
		return errors.New("s3 put reader is required")
	}
	objectKey, err := p.objectKey(key)
	if err != nil {
		return err
	}
	if _, err := p.client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(objectKey),
		Body:   reader,
	}); err != nil {
		return fmt.Errorf("s3 put object %q: %w", objectKey, err)
	}
	return nil
}

func (p *Provider) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	objectKey, err := p.objectKey(key)
	if err != nil {
		return nil, err
	}
	output, err := p.client.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("s3 get object %q: %w", objectKey, storage.ErrNotFound)
		}
		return nil, fmt.Errorf("s3 get object %q: %w", objectKey, err)
	}
	if output == nil || output.Body == nil {
		return nil, fmt.Errorf("s3 get object %q returned an empty body", objectKey)
	}
	return output.Body, nil
}

func isNotFound(err error) bool {
	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return true
	}
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return true
	}
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(apiErr.ErrorCode())) {
	case "nosuchkey", "notfound", "no_such_key":
		return true
	default:
		return false
	}
}

func (p *Provider) Delete(ctx context.Context, key string) error {
	objectKey, err := p.objectKey(key)
	if err != nil {
		return err
	}
	if _, err := p.client.DeleteObject(ctx, &awss3.DeleteObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(objectKey),
	}); err != nil {
		return fmt.Errorf("s3 delete object %q: %w", objectKey, err)
	}
	return nil
}

// AccessPath deliberately returns no URL. Consumers should resolve objects by
// content hash through media.Service, or use a signed Memoh media endpoint when
// an external platform must fetch the bytes.
func (*Provider) AccessPath(context.Context, string) string {
	return ""
}

// PrefixListingAuthoritative marks successful S3 listings as complete.
func (*Provider) PrefixListingAuthoritative() {}

// ListPrefix returns logical storage keys without the provider-level prefix.
func (p *Provider) ListPrefix(ctx context.Context, prefix string) ([]string, error) {
	objectPrefix, err := p.objectKey(prefix)
	if err != nil {
		return nil, err
	}

	var (
		continuationToken *string
		keys              []string
	)
	for {
		output, listErr := p.client.ListObjectsV2(ctx, &awss3.ListObjectsV2Input{
			Bucket:            aws.String(p.bucket),
			Prefix:            aws.String(objectPrefix),
			ContinuationToken: continuationToken,
		})
		if listErr != nil {
			return nil, fmt.Errorf("s3 list objects %q: %w", objectPrefix, listErr)
		}
		if output == nil {
			return nil, fmt.Errorf("s3 list objects %q returned an empty response", objectPrefix)
		}
		for _, object := range output.Contents {
			key := strings.TrimSpace(aws.ToString(object.Key))
			if key == "" {
				continue
			}
			logicalKey, ok := p.logicalKey(key)
			if ok {
				keys = append(keys, logicalKey)
			}
		}
		if !aws.ToBool(output.IsTruncated) {
			break
		}
		if strings.TrimSpace(aws.ToString(output.NextContinuationToken)) == "" {
			return nil, fmt.Errorf("s3 list objects %q is truncated without a continuation token", objectPrefix)
		}
		continuationToken = output.NextContinuationToken
	}
	return keys, nil
}

func (p *Provider) objectKey(key string) (string, error) {
	clean, err := cleanKey(key)
	if err != nil {
		return "", err
	}
	if p.prefix == "" {
		return clean, nil
	}
	return path.Join(p.prefix, clean), nil
}

func (p *Provider) logicalKey(objectKey string) (string, bool) {
	if p.prefix == "" {
		clean, err := cleanKey(objectKey)
		return clean, err == nil
	}
	prefix := p.prefix + "/"
	if !strings.HasPrefix(objectKey, prefix) {
		return "", false
	}
	clean, err := cleanKey(strings.TrimPrefix(objectKey, prefix))
	return clean, err == nil
}

func cleanPrefix(prefix string) (string, error) {
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if prefix == "" {
		return "", nil
	}
	return cleanKey(prefix)
}

func cleanKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("s3 storage key is required")
	}
	if strings.Contains(key, "\\") || strings.HasPrefix(key, "/") {
		return "", fmt.Errorf("invalid s3 storage key %q", key)
	}
	clean := path.Clean(key)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean != key {
		return "", fmt.Errorf("invalid s3 storage key %q", key)
	}
	return clean, nil
}
