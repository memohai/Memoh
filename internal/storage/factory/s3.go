// Package factory constructs configured storage providers without coupling
// application composition roots to individual SDK clients.
package factory

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/memohai/memoh/internal/config"
	s3provider "github.com/memohai/memoh/internal/storage/providers/s3"
)

// NewS3 creates a private S3-compatible provider from application config.
func NewS3(cfg config.S3StorageConfig) (*s3provider.Provider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	credentials := aws.NewCredentialsCache(aws.CredentialsProviderFunc(
		func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{
				AccessKeyID:     strings.TrimSpace(cfg.AccessKeyID),
				SecretAccessKey: strings.TrimSpace(cfg.SecretAccessKey),
				Source:          "memoh-storage-config",
			}, nil
		},
	))
	client := awss3.New(awss3.Options{
		BaseEndpoint: aws.String(strings.TrimSpace(cfg.Endpoint)),
		Region:       cfg.RegionOrDefault(),
		Credentials:  credentials,
		UsePathStyle: cfg.PathStyle,
	})
	provider, err := s3provider.New(client, strings.TrimSpace(cfg.Bucket), cfg.Prefix)
	if err != nil {
		return nil, fmt.Errorf("create s3 storage provider: %w", err)
	}
	return provider, nil
}
