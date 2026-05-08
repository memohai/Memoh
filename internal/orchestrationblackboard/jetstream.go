package orchestrationblackboard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// DefaultBucketName is the bucket NewJetStreamStore uses unless the caller
// overrides it. One bucket holds the entire orchestration blackboard; runs
// and tasks are namespaced inside via Key.String().
const DefaultBucketName = "MEMOH_ORCH_BLACKBOARD"

// defaultHistory keeps a single revision per key on the server. The
// blackboard treats CAS revisions as the ordering primitive; the rebuild
// path can rebuild older state from Postgres if it ever needs to.
const defaultHistory = 1

// JetStreamConfig captures the runtime knobs needed to provision the
// blackboard KV bucket and connect the NATS client.
type JetStreamConfig struct {
	URL             string
	Token           string `json:"-"`
	User            string
	Password        string `json:"-"`
	CredentialsFile string

	// Bucket overrides the default KV bucket name. Useful when a single
	// NATS deployment hosts multiple Memoh environments side by side.
	Bucket string

	// Replicas controls the per-bucket replica count. JetStream requires
	// it to be at least 1.
	Replicas int

	// TTL bounds how long values live. Zero means values live until they
	// are explicitly overwritten or deleted; recovery still works because
	// Postgres remains authoritative.
	TTL time.Duration

	// History sets the per-key revision history kept by the bucket.
	// Default is 1; raise it only when off-line audit replay is wanted.
	History uint8

	// ConnectionName surfaces on the NATS server so operators can attach
	// blackboard activity to the originating component.
	ConnectionName string
}

// JetStreamStore implements Store on top of a NATS KV bucket. Revisions
// are passed straight through to JetStream so CompareAndSwap maps to the
// bucket's optimistic-concurrency primitives without any client-side
// emulation.
type JetStreamStore struct {
	logger *slog.Logger
	cfg    JetStreamConfig

	conn *nats.Conn
	js   jetstream.JetStream
	kv   jetstream.KeyValue

	mu     sync.Mutex
	closed bool
}

// NewJetStreamStore connects to the configured NATS server, ensures the
// blackboard bucket exists, and returns a ready-to-use store. Callers
// must invoke Close on shutdown to drain the connection.
func NewJetStreamStore(ctx context.Context, logger *slog.Logger, cfg JetStreamConfig) (*JetStreamStore, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if strings.TrimSpace(cfg.URL) == "" {
		return nil, errors.New("orchestrationblackboard: jetstream url is required")
	}
	if strings.TrimSpace(cfg.Bucket) == "" {
		cfg.Bucket = DefaultBucketName
	}
	if cfg.Replicas <= 0 {
		cfg.Replicas = 1
	}
	if cfg.History == 0 {
		cfg.History = defaultHistory
	}
	if strings.TrimSpace(cfg.ConnectionName) == "" {
		cfg.ConnectionName = "memoh-orchestration-blackboard"
	}

	natsOpts := []nats.Option{
		nats.Name(cfg.ConnectionName),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
	}
	if v := strings.TrimSpace(cfg.Token); v != "" {
		natsOpts = append(natsOpts, nats.Token(v))
	}
	if v := strings.TrimSpace(cfg.User); v != "" {
		natsOpts = append(natsOpts, nats.UserInfo(v, cfg.Password))
	}
	if v := strings.TrimSpace(cfg.CredentialsFile); v != "" {
		natsOpts = append(natsOpts, nats.UserCredentials(v))
	}

	conn, err := nats.Connect(cfg.URL, natsOpts...)
	if err != nil {
		return nil, fmt.Errorf("orchestrationblackboard: connect nats: %w", err)
	}

	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("orchestrationblackboard: jetstream context: %w", err)
	}

	bucketCfg := jetstream.KeyValueConfig{
		Bucket:      cfg.Bucket,
		Description: "Memoh orchestration blackboard runtime view",
		History:     cfg.History,
		Storage:     jetstream.FileStorage,
		Replicas:    cfg.Replicas,
		TTL:         cfg.TTL,
	}
	kv, err := js.CreateOrUpdateKeyValue(ctx, bucketCfg)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("orchestrationblackboard: ensure bucket %q: %w", cfg.Bucket, err)
	}

	store := &JetStreamStore{
		logger: logger.With(slog.String("component", "orchestrationblackboard.jetstream")),
		cfg:    cfg,
		conn:   conn,
		js:     js,
		kv:     kv,
	}
	store.logger.Info("blackboard jetstream store ready",
		slog.String("url", cfg.URL),
		slog.String("bucket", cfg.Bucket),
	)
	return store, nil
}

// Get implements Store.
func (s *JetStreamStore) Get(ctx context.Context, key Key) (Entry, error) {
	if err := key.Validate(); err != nil {
		return Entry{}, err
	}
	if err := s.checkOpen(); err != nil {
		return Entry{}, err
	}
	entry, err := s.kv.Get(ctx, key.String())
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return Entry{}, ErrNotFound
		}
		return Entry{}, fmt.Errorf("orchestrationblackboard: kv get %s: %w", key.String(), err)
	}
	value, decErr := DecodeValue(entry.Value())
	if decErr != nil {
		return Entry{}, decErr
	}
	return Entry{Key: key, Value: value, Revision: Revision(entry.Revision())}, nil
}

// Put implements Store.
func (s *JetStreamStore) Put(ctx context.Context, key Key, value Value) (Revision, error) {
	if err := key.Validate(); err != nil {
		return 0, err
	}
	if err := value.Validate(); err != nil {
		return 0, err
	}
	if err := s.checkOpen(); err != nil {
		return 0, err
	}
	payload, err := value.Encode()
	if err != nil {
		return 0, err
	}
	rev, err := s.kv.Put(ctx, key.String(), payload)
	if err != nil {
		return 0, fmt.Errorf("orchestrationblackboard: kv put %s: %w", key.String(), err)
	}
	return Revision(rev), nil
}

// CompareAndSwap implements Store. Pass expected == 0 to insert when the
// caller believes the key is unset; CompareAndSwap maps that to KV
// Create. Non-zero expected maps to KV Update. CAS conflicts surface as
// ErrRevisionConflict regardless of which path was taken.
func (s *JetStreamStore) CompareAndSwap(ctx context.Context, key Key, expected Revision, value Value) (Revision, error) {
	if err := key.Validate(); err != nil {
		return 0, err
	}
	if err := value.Validate(); err != nil {
		return 0, err
	}
	if err := s.checkOpen(); err != nil {
		return 0, err
	}
	payload, err := value.Encode()
	if err != nil {
		return 0, err
	}
	if expected == 0 {
		rev, err := s.kv.Create(ctx, key.String(), payload)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyExists) || isWrongLastSequence(err) {
				return 0, ErrRevisionConflict
			}
			return 0, fmt.Errorf("orchestrationblackboard: kv create %s: %w", key.String(), err)
		}
		return Revision(rev), nil
	}
	rev, err := s.kv.Update(ctx, key.String(), payload, uint64(expected))
	if err != nil {
		if isWrongLastSequence(err) || errors.Is(err, jetstream.ErrKeyNotFound) {
			return 0, ErrRevisionConflict
		}
		return 0, fmt.Errorf("orchestrationblackboard: kv update %s: %w", key.String(), err)
	}
	return Revision(rev), nil
}

// Delete implements Store. NATS KV records a tombstone; callers see
// ErrNotFound on subsequent Get and the entry is hidden from List.
func (s *JetStreamStore) Delete(ctx context.Context, key Key) error {
	if err := key.Validate(); err != nil {
		return err
	}
	if err := s.checkOpen(); err != nil {
		return err
	}
	if err := s.kv.Delete(ctx, key.String()); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("orchestrationblackboard: kv delete %s: %w", key.String(), err)
	}
	return nil
}

// List implements Store. The implementation opens a filtered watcher
// scoped to prefix and prefix.>, reads until JetStream signals
// initial-sync complete (a nil entry on the channel), and returns the
// collected snapshot.
func (s *JetStreamStore) List(ctx context.Context, prefix Key) ([]Entry, error) {
	if err := prefix.Validate(); err != nil {
		return nil, err
	}
	if err := s.checkOpen(); err != nil {
		return nil, err
	}
	pattern := []string{prefix.String(), prefix.String() + ".>"}
	watcher, err := s.kv.WatchFiltered(ctx, pattern, jetstream.IgnoreDeletes())
	if err != nil {
		return nil, fmt.Errorf("orchestrationblackboard: kv watch: %w", err)
	}
	defer func() { _ = watcher.Stop() }()

	var entries []Entry
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case entry, ok := <-watcher.Updates():
			if !ok {
				return entries, nil
			}
			if entry == nil {
				return entries, nil
			}
			value, decErr := DecodeValue(entry.Value())
			if decErr != nil {
				return nil, decErr
			}
			parsed, parseErr := parseStoredKey(entry.Key())
			if parseErr != nil {
				s.logger.Warn("blackboard list skipped malformed key",
					slog.String("key", entry.Key()),
					slog.Any("error", parseErr))
				continue
			}
			entries = append(entries, Entry{Key: parsed, Value: value, Revision: Revision(entry.Revision())})
		}
	}
}

// Close implements Store.
func (s *JetStreamStore) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	if s.conn != nil {
		_ = s.conn.Drain()
	}
	return nil
}

func (s *JetStreamStore) checkOpen() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	return nil
}

// isWrongLastSequence detects the JetStream API error returned when a CAS
// update is rejected because the server's view of the key has moved past
// the supplied revision. The API error code lives in jetstream.APIError.
func isWrongLastSequence(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *jetstream.APIError
	if errors.As(err, &apiErr) {
		if apiErr.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequence {
			return true
		}
	}
	return strings.Contains(err.Error(), "wrong last sequence")
}

// parseStoredKey reverses Key.String. It is used by List to materialise
// the canonical Key from the raw KV key emitted by NATS.
func parseStoredKey(s string) (Key, error) {
	parts := strings.Split(s, ".")
	if len(parts) < 4 || parts[0] != "bb" {
		return Key{}, fmt.Errorf("orchestrationblackboard: invalid stored key %q", s)
	}
	key := Key{
		Scope:     Scope(parts[1]),
		OwnerID:   parts[2],
		Namespace: Namespace(parts[3]),
	}
	if len(parts) > 4 {
		key.Path = append([]string(nil), parts[4:]...)
	}
	if err := key.Validate(); err != nil {
		return Key{}, err
	}
	return key, nil
}
