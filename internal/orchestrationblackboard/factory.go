package orchestrationblackboard

import (
	"context"
	"log/slog"
	"strings"
)

// FactoryConfig describes which backend to instantiate. When URL is empty
// the factory returns an InMemoryStore so single-process and unit-test
// callers can use the same wiring as production deployments.
type FactoryConfig struct {
	URL             string
	Token           string `json:"-"`
	User            string
	Password        string `json:"-"`
	CredentialsFile string
	Bucket          string
	Replicas        int
	ConnectionName  string
}

// New returns a Store appropriate for the supplied configuration. The
// orchestrator wires this from the global [nats] config block; passing
// an empty URL keeps tests and stand-alone CLI tooling on the in-memory
// backend without changing call sites.
func New(ctx context.Context, logger *slog.Logger, cfg FactoryConfig) (Store, error) {
	if strings.TrimSpace(cfg.URL) == "" {
		if logger != nil {
			logger.Info("blackboard backend selected", slog.String("backend", "in-memory"))
		}
		return NewInMemoryStore(), nil
	}
	jsCfg := JetStreamConfig{
		URL:             cfg.URL,
		Token:           cfg.Token,
		User:            cfg.User,
		Password:        cfg.Password,
		CredentialsFile: cfg.CredentialsFile,
		Bucket:          cfg.Bucket,
		Replicas:        cfg.Replicas,
		ConnectionName:  cfg.ConnectionName,
	}
	return NewJetStreamStore(ctx, logger, jsCfg)
}
