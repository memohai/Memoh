package sessionruntime

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/memohai/memoh/internal/conversation"
)

type RedisOptions struct {
	URL       string
	KeyPrefix string
	StateTTL  time.Duration
}

type RedisBackend struct {
	client    *redis.Client
	keyPrefix string
	stateTTL  time.Duration

	subscriptionsMu sync.Mutex
	subscriptions   map[uint64]context.CancelFunc
	nextSubID       uint64
	subscriptionsWG sync.WaitGroup
	closed          bool
	closeOnce       sync.Once
	closeDone       chan struct{}
	closeErr        error
}

var renewRedisLeaseScript = redis.NewScript(`
local current_state = redis.call('GET', KEYS[1])
local current_ref = redis.call('GET', KEYS[2])
if not current_state or not current_ref then
  return -1
end
if current_state ~= ARGV[1] or current_ref ~= ARGV[2] then
  return 0
end
local now = redis.call('TIME')
local now_ms = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)
local expires_at_ms = tonumber(ARGV[5])
if not expires_at_ms or expires_at_ms <= now_ms then
  return -1
end
redis.call('SET', KEYS[1], ARGV[3], 'PX', ARGV[6])
redis.call('SET', KEYS[2], ARGV[4], 'PXAT', ARGV[5])
redis.call('ZADD', KEYS[3], ARGV[5], ARGV[7])
return 1
`)

var deleteRedisStreamRefScript = redis.NewScript(`
local current_ref = redis.call('GET', KEYS[1])
if not current_ref or current_ref ~= ARGV[1] then
  return 0
end
redis.call('DEL', KEYS[1])
local state_data = redis.call('GET', KEYS[3])
if not state_data then
  redis.call('ZREM', KEYS[2], ARGV[2])
  return 1
end
local ok_ref, expected_ref = pcall(cjson.decode, ARGV[1])
local ok_state, state = pcall(cjson.decode, state_data)
if ok_ref and ok_state and state.current_run_view then
  local run = state.current_run_view
  if run.stream_id == expected_ref.stream_id and run.generation == expected_ref.generation then
    redis.call('ZREM', KEYS[2], ARGV[2])
  end
end
return 1
`)

var listExpiredRedisRunsScript = redis.NewScript(`
local now = redis.call('TIME')
local now_ms = tonumber(now[1]) * 1000 + math.floor(tonumber(now[2]) / 1000)
local members = redis.call('ZRANGEBYSCORE', KEYS[1], '-inf', now_ms, 'LIMIT', 0, ARGV[1])
local claim_until_ms = now_ms + tonumber(ARGV[3])
local active = {}
for _, member in ipairs(members) do
  local ok_key, key = pcall(cjson.decode, member)
  local keep = false
  if ok_key and key.bot_id and key.session_id then
    local state_data = redis.call('GET', ARGV[2] .. key.bot_id .. ':' .. key.session_id)
    if state_data then
      local ok_state, state = pcall(cjson.decode, state_data)
      if ok_state and state.current_run_view then
        local status = state.current_run_view.status
        keep = status == 'admitting' or status == 'running' or status == 'aborting'
      end
    end
  end
  if keep then
    redis.call('ZADD', KEYS[1], claim_until_ms, member)
    table.insert(active, member)
  else
    redis.call('ZREM', KEYS[1], member)
  end
end
return active
`)

var appendRedisNormalizedMessageScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[4]) == 0 then
  return {0}
end
local current_ref = redis.call('GET', KEYS[1])
if not current_ref or current_ref ~= ARGV[1] or redis.call('PTTL', KEYS[1]) <= 0 then
  return {0}
end
local content = redis.call('HGET', KEYS[2], ARGV[2])
if not content then
  return {2}
end
local epoch = redis.call('HGET', KEYS[3], 'epoch')
local current_seq = redis.call('HGET', KEYS[3], 'seq')
if not epoch or not current_seq then
  return {2}
end
local now = redis.call('TIME')
local seq = redis.call('HINCRBY', KEYS[3], 'seq', 1)
redis.call('HSET', KEYS[2], ARGV[2], content .. ARGV[3])
redis.call('HSET', KEYS[3], 'updated_at_seconds', now[1], 'updated_at_microseconds', now[2])
redis.call('PEXPIRE', KEYS[4], ARGV[4])
redis.call('PEXPIRE', KEYS[2], ARGV[4])
redis.call('PEXPIRE', KEYS[3], ARGV[4])
return {1, tostring(seq), epoch, now[1], now[2]}
`)

var validateRedisRunOwnershipScript = redis.NewScript(`
local current_state = redis.call('GET', KEYS[1])
local current_ref = redis.call('GET', KEYS[2])
if not current_state or not current_ref then
  return 0
end

local ok_ref, ref = pcall(cjson.decode, current_ref)
local ok_state, state = pcall(cjson.decode, current_state)
if not ok_ref or not ok_state or not state.current_run_view then
  return 0
end

local run = state.current_run_view
if ref.bot_id ~= ARGV[1] or ref.session_id ~= ARGV[2] or
   ref.stream_id ~= ARGV[3] or ref.owner_id ~= ARGV[4] or ref.generation ~= ARGV[5] or
   state.bot_id ~= ARGV[1] or state.session_id ~= ARGV[2] or
   run.stream_id ~= ARGV[3] or run.owner_id ~= ARGV[4] or run.generation ~= ARGV[5] then
  return 0
end

if run.status ~= 'admitting' and run.status ~= 'running' and run.status ~= 'aborting' then
  return 0
end

-- PTTL and TIME are evaluated by Redis in this atomic script, so expiry cannot
-- pass between separate client reads. TIME is intentionally sampled here to
-- keep the ownership decision on the Redis server clock.
local now = redis.call('TIME')
local ttl = redis.call('PTTL', KEYS[2])
if not now or ttl <= 0 then
  return 0
end
return 1
`)

func NewRedisBackend(ctx context.Context, opts RedisOptions) (*RedisBackend, error) {
	backend, err := newRedisBackend(opts)
	if err != nil {
		return nil, err
	}
	if err := backend.CheckHealth(ctx); err != nil {
		_ = backend.Close()
		return nil, err
	}
	return backend, nil
}

var errRedisBackendClosed = errors.New("session runtime redis backend is closed")

const redisTransactionMaxAttempts = 64

const redisExpiredRunClaimTTL = 2 * time.Second

func newRedisBackend(opts RedisOptions) (*RedisBackend, error) {
	redisOpts, err := redis.ParseURL(strings.TrimSpace(opts.URL))
	if err != nil {
		return nil, err
	}
	redisOpts.ContextTimeoutEnabled = true
	client := redis.NewClient(redisOpts)
	ttl := opts.StateTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	prefix := strings.TrimSpace(opts.KeyPrefix)
	if prefix == "" {
		prefix = "memoh:session_runtime:"
	}
	return &RedisBackend{
		client:        client,
		keyPrefix:     prefix,
		stateTTL:      ttl,
		subscriptions: make(map[uint64]context.CancelFunc),
		closeDone:     make(chan struct{}),
	}, nil
}

func (b *RedisBackend) registerSubscription(cancel context.CancelFunc) (uint64, bool) {
	b.subscriptionsMu.Lock()
	defer b.subscriptionsMu.Unlock()
	if b.closed {
		return 0, false
	}
	b.nextSubID++
	id := b.nextSubID
	b.subscriptions[id] = cancel
	b.subscriptionsWG.Add(1)
	return id, true
}

func (b *RedisBackend) unregisterSubscription(id uint64) {
	b.subscriptionsMu.Lock()
	delete(b.subscriptions, id)
	b.subscriptionsMu.Unlock()
	b.subscriptionsWG.Done()
}

func (b *RedisBackend) CheckHealth(ctx context.Context) error {
	return b.client.Ping(ctx).Err()
}

func (b *RedisBackend) Now(ctx context.Context) (time.Time, error) {
	return b.client.Time(ctx).Result()
}

func (b *RedisBackend) Load(ctx context.Context, key Key) (Snapshot, bool, error) {
	stateKey := b.stateKey(key)
	contentKey := b.contentKey(key)
	revisionKey := b.revisionKey(key)
	for attempt := 0; attempt < redisTransactionMaxAttempts; attempt++ {
		var snapshot Snapshot
		var ok bool
		err := b.client.Watch(ctx, func(tx *redis.Tx) error {
			var err error
			snapshot, ok, err = loadRedisSnapshot(ctx, tx, stateKey, contentKey, revisionKey)
			if err != nil {
				return err
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Exists(ctx, stateKey)
				return nil
			})
			return err
		}, stateKey, contentKey, revisionKey)
		if errors.Is(err, redis.TxFailedErr) {
			if retryErr := waitRedisTransactionRetry(ctx, attempt); retryErr != nil {
				return Snapshot{}, false, retryErr
			}
			continue
		}
		return snapshot, ok, err
	}
	return Snapshot{}, false, ErrBackendConflict
}

func (b *RedisBackend) Update(ctx context.Context, key Key, update SnapshotUpdate) (Snapshot, bool, error) {
	if update == nil {
		return Snapshot{}, false, errors.New("snapshot update is required")
	}
	stateKey := b.stateKey(key)
	contentKey := b.contentKey(key)
	revisionKey := b.revisionKey(key)
	for attempt := 0; attempt < redisTransactionMaxAttempts; attempt++ {
		var updated Snapshot
		var changed bool
		err := b.client.Watch(ctx, func(tx *redis.Tx) error {
			// current is freshly unmarshaled per attempt and next is
			// transaction-local, so neither needs a defensive clone.
			current, ok, err := loadRedisSnapshot(ctx, tx, stateKey, contentKey, revisionKey)
			if err != nil {
				return err
			}
			next, apply, err := update(current, ok)
			if err != nil {
				return err
			}
			if !apply {
				updated = next
				changed = false
				return nil
			}
			next.Queue = nonNilQueue(next.Queue)
			prepared, err := prepareRedisSnapshot(next)
			if err != nil {
				return err
			}
			if _, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				writeRedisSnapshot(ctx, pipe, stateKey, contentKey, revisionKey, prepared, b.stateTTL)
				return nil
			}); err != nil {
				return err
			}
			updated = next
			changed = true
			return nil
		}, stateKey, contentKey, revisionKey)
		if errors.Is(err, redis.TxFailedErr) {
			if retryErr := waitRedisTransactionRetry(ctx, attempt); retryErr != nil {
				return Snapshot{}, false, retryErr
			}
			continue
		}
		return updated, changed, err
	}
	return Snapshot{}, false, ErrBackendConflict
}

func (b *RedisBackend) UpdateActiveRun(ctx context.Context, key Key, streamID, generation string, update ActiveRunUpdate) (Snapshot, bool, error) {
	if update == nil {
		return Snapshot{}, false, errors.New("active run update is required")
	}
	streamID = strings.TrimSpace(streamID)
	generation = strings.TrimSpace(generation)
	if streamID == "" || generation == "" {
		return Snapshot{}, false, errors.New("stream_id and generation are required")
	}
	stateKey := b.stateKey(key)
	contentKey := b.contentKey(key)
	revisionKey := b.revisionKey(key)
	streamKey := b.streamKey(key, streamID)
	for attempt := 0; attempt < redisTransactionMaxAttempts; attempt++ {
		var updated Snapshot
		var changed bool
		err := b.client.Watch(ctx, func(tx *redis.Tx) error {
			now, err := tx.Time(ctx).Result()
			if err != nil {
				return err
			}
			ref, ok, err := loadRedisStreamRef(ctx, tx, streamKey)
			if err != nil {
				return err
			}
			current, stateOK, err := loadRedisSnapshot(ctx, tx, stateKey, contentKey, revisionKey)
			if err != nil {
				return err
			}
			if !ok || !stateOK || current.CurrentRunView == nil {
				return ErrRunOwnershipLost
			}
			run := current.CurrentRunView
			if run.StreamID != streamID || run.Generation != generation || ref.StreamID != streamID || ref.Generation != generation || ref.OwnerID != run.OwnerID || ref.BotID != key.BotID || ref.SessionID != key.SessionID || !isActiveRunStatus(run.Status) || run.OwnerLeaseExpiresAt == nil || !now.Before(*run.OwnerLeaseExpiresAt) {
				return ErrRunOwnershipLost
			}
			next, apply, err := update(current, now)
			if err != nil {
				return err
			}
			if !apply {
				updated = next
				changed = false
				return nil
			}
			next.Queue = nonNilQueue(next.Queue)
			prepared, err := prepareRedisSnapshot(next)
			if err != nil {
				return err
			}
			if _, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				writeRedisSnapshot(ctx, pipe, stateKey, contentKey, revisionKey, prepared, b.stateTTL)
				return nil
			}); err != nil {
				return err
			}
			updated = next
			changed = true
			return nil
		}, stateKey, contentKey, revisionKey, streamKey)
		if errors.Is(err, redis.TxFailedErr) {
			if retryErr := waitRedisTransactionRetry(ctx, attempt); retryErr != nil {
				return Snapshot{}, false, retryErr
			}
			continue
		}
		return updated, changed, err
	}
	return Snapshot{}, false, ErrBackendConflict
}

func (b *RedisBackend) AppendActiveRunMessage(ctx context.Context, key Key, ref StreamRef, messageAppend RuntimeMessageAppend) (RuntimeRevision, bool, error) {
	ref.BotID = strings.TrimSpace(ref.BotID)
	ref.SessionID = strings.TrimSpace(ref.SessionID)
	ref.StreamID = strings.TrimSpace(ref.StreamID)
	ref.OwnerID = strings.TrimSpace(ref.OwnerID)
	ref.Generation = strings.TrimSpace(ref.Generation)
	if ref.BotID != strings.TrimSpace(key.BotID) || ref.SessionID != strings.TrimSpace(key.SessionID) || ref.StreamID == "" || ref.OwnerID == "" || ref.Generation == "" {
		return RuntimeRevision{}, false, ErrRunOwnershipLost
	}
	if messageAppend.Content == "" {
		return RuntimeRevision{}, false, nil
	}
	if messageAppend.Type != conversation.UIMessageText && messageAppend.Type != conversation.UIMessageReasoning {
		return RuntimeRevision{}, false, nil
	}
	refData, err := marshalRuntimeJSON(ref)
	if err != nil {
		return RuntimeRevision{}, false, err
	}
	contentField := redisMessageContentField(ref.Generation, conversation.UIMessage{
		ID:   messageAppend.ID,
		Type: messageAppend.Type,
	})
	result, err := appendRedisNormalizedMessageScript.Run(
		ctx,
		b.client,
		[]string{b.streamKey(key, ref.StreamID), b.contentKey(key), b.revisionKey(key), b.stateKey(key)},
		refData,
		contentField,
		messageAppend.Content,
		b.stateTTL.Milliseconds(),
	).Slice()
	if err != nil {
		return RuntimeRevision{}, false, err
	}
	if len(result) == 0 {
		return RuntimeRevision{}, false, errors.New("runtime append script returned no status")
	}
	status, err := redisScriptInt64(result[0])
	if err != nil {
		return RuntimeRevision{}, false, fmt.Errorf("decode runtime append status: %w", err)
	}
	switch status {
	case 0:
		return RuntimeRevision{}, false, ErrRunOwnershipLost
	case 2:
		return RuntimeRevision{}, false, nil
	case 1:
	default:
		return RuntimeRevision{}, false, fmt.Errorf("unexpected runtime append status %d", status)
	}
	if len(result) != 5 {
		return RuntimeRevision{}, false, fmt.Errorf("runtime append script returned %d values", len(result))
	}
	seq, err := redisScriptInt64(result[1])
	if err != nil {
		return RuntimeRevision{}, false, fmt.Errorf("decode runtime append seq: %w", err)
	}
	epoch := strings.TrimSpace(fmt.Sprint(result[2]))
	seconds, err := redisScriptInt64(result[3])
	if err != nil {
		return RuntimeRevision{}, false, fmt.Errorf("decode runtime append seconds: %w", err)
	}
	microseconds, err := redisScriptInt64(result[4])
	if err != nil {
		return RuntimeRevision{}, false, fmt.Errorf("decode runtime append microseconds: %w", err)
	}
	if epoch == "" || seq <= 0 {
		return RuntimeRevision{}, false, errors.New("runtime append revision is invalid")
	}
	return RuntimeRevision{
		Epoch:     epoch,
		Seq:       seq,
		UpdatedAt: time.Unix(seconds, microseconds*int64(time.Microsecond)).UTC(),
	}, true, nil
}

func (b *RedisBackend) ReleaseRun(ctx context.Context, key Key, ref StreamRef, update ActiveRunUpdate) (Snapshot, bool, error) {
	if update == nil {
		return Snapshot{}, false, errors.New("active run update is required")
	}
	ref.BotID = strings.TrimSpace(ref.BotID)
	ref.SessionID = strings.TrimSpace(ref.SessionID)
	ref.StreamID = strings.TrimSpace(ref.StreamID)
	ref.OwnerID = strings.TrimSpace(ref.OwnerID)
	ref.Generation = strings.TrimSpace(ref.Generation)
	if ref.BotID != strings.TrimSpace(key.BotID) || ref.SessionID != strings.TrimSpace(key.SessionID) || ref.StreamID == "" || ref.OwnerID == "" || ref.Generation == "" {
		return Snapshot{}, false, ErrRunOwnershipLost
	}
	stateKey := b.stateKey(key)
	contentKey := b.contentKey(key)
	revisionKey := b.revisionKey(key)
	streamKey := b.streamKey(key, ref.StreamID)
	for attempt := 0; attempt < redisTransactionMaxAttempts; attempt++ {
		var updated Snapshot
		var changed bool
		err := b.client.Watch(ctx, func(tx *redis.Tx) error {
			now, err := tx.Time(ctx).Result()
			if err != nil {
				return err
			}
			storedRef, ok, err := loadRedisStreamRef(ctx, tx, streamKey)
			if err != nil {
				return err
			}
			current, stateOK, err := loadRedisSnapshot(ctx, tx, stateKey, contentKey, revisionKey)
			if err != nil {
				return err
			}
			if !ok || !stateOK || current.CurrentRunView == nil {
				return ErrRunOwnershipLost
			}
			run := current.CurrentRunView
			if storedRef != ref || run.StreamID != ref.StreamID || run.Generation != ref.Generation || run.OwnerID != ref.OwnerID || !isActiveRunStatus(run.Status) || run.OwnerLeaseExpiresAt == nil || !now.Before(*run.OwnerLeaseExpiresAt) {
				return ErrRunOwnershipLost
			}
			next, apply, err := update(current, now)
			if err != nil {
				return err
			}
			if !apply {
				updated = next
				changed = false
				return nil
			}
			next.Queue = nonNilQueue(next.Queue)
			prepared, err := prepareRedisSnapshot(next)
			if err != nil {
				return err
			}
			if _, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				writeRedisSnapshot(ctx, pipe, stateKey, contentKey, revisionKey, prepared, b.stateTTL)
				pipe.Del(ctx, streamKey)
				pipe.ZRem(ctx, b.runLeaseIndexKey(), redisRunLeaseMember(key))
				return nil
			}); err != nil {
				return err
			}
			updated = next
			changed = true
			return nil
		}, stateKey, contentKey, revisionKey, streamKey)
		if errors.Is(err, redis.TxFailedErr) {
			if retryErr := waitRedisTransactionRetry(ctx, attempt); retryErr != nil {
				return Snapshot{}, false, retryErr
			}
			continue
		}
		return updated, changed, err
	}
	return Snapshot{}, false, ErrBackendConflict
}

func (b *RedisBackend) StartRun(ctx context.Context, key Key, ref StreamRef, update ActiveRunUpdate) (Snapshot, bool, error) {
	if update == nil {
		return Snapshot{}, false, errors.New("snapshot update is required")
	}
	streamID := strings.TrimSpace(ref.StreamID)
	ref.Generation = strings.TrimSpace(ref.Generation)
	if streamID == "" || ref.Generation == "" {
		return Snapshot{}, false, errors.New("stream_id and generation are required")
	}
	ref.StreamID = streamID
	refData, err := marshalRuntimeJSON(ref)
	if err != nil {
		return Snapshot{}, false, err
	}
	stateKey := b.stateKey(key)
	contentKey := b.contentKey(key)
	revisionKey := b.revisionKey(key)
	streamKey := b.streamKey(key, streamID)
	for attempt := 0; attempt < redisTransactionMaxAttempts; attempt++ {
		var updated Snapshot
		var changed bool
		err := b.client.Watch(ctx, func(tx *redis.Tx) error {
			now, err := tx.Time(ctx).Result()
			if err != nil {
				return err
			}
			if existing, ok, err := loadRedisStreamRef(ctx, tx, streamKey); err != nil {
				return err
			} else if ok {
				return fmt.Errorf("stream_id %q is already registered for session %q", streamID, existing.SessionID)
			}
			current, _, err := loadRedisSnapshot(ctx, tx, stateKey, contentKey, revisionKey)
			if err != nil {
				return err
			}
			next, apply, err := update(current, now)
			if err != nil {
				return err
			}
			if !apply {
				updated = next
				changed = false
				return nil
			}
			next.Queue = nonNilQueue(next.Queue)
			prepared, err := prepareRedisSnapshot(next)
			if err != nil {
				return err
			}
			if _, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				writeRedisSnapshot(ctx, pipe, stateKey, contentKey, revisionKey, prepared, b.stateTTL)
				pipe.Set(ctx, streamKey, refData, 0)
				leaseExpiry := streamLeaseExpiry(next, streamID, now.Add(b.stateTTL))
				pipe.PExpireAt(ctx, streamKey, leaseExpiry)
				pipe.ZAdd(ctx, b.runLeaseIndexKey(), redis.Z{Score: float64(leaseExpiry.UnixMilli()), Member: redisRunLeaseMember(key)})
				return nil
			}); err != nil {
				return err
			}
			updated = next
			changed = true
			return nil
		}, stateKey, contentKey, revisionKey, streamKey)
		if errors.Is(err, redis.TxFailedErr) {
			if retryErr := waitRedisTransactionRetry(ctx, attempt); retryErr != nil {
				return Snapshot{}, false, retryErr
			}
			continue
		}
		return updated, changed, err
	}
	return Snapshot{}, false, ErrBackendConflict
}

func (b *RedisBackend) RenewLease(ctx context.Context, key Key, streamID, ownerID, generation string, renewedAt, expiresAt time.Time) error {
	streamID = strings.TrimSpace(streamID)
	ownerID = strings.TrimSpace(ownerID)
	generation = strings.TrimSpace(generation)
	if streamID == "" || ownerID == "" || generation == "" {
		return nil
	}
	if renewedAt.IsZero() || !renewedAt.Before(expiresAt) {
		return ErrRunOwnershipLost
	}
	stateKey := b.stateKey(key)
	streamKey := b.streamKey(key, streamID)
	for attempt := 0; attempt < redisTransactionMaxAttempts; attempt++ {
		stateData, err := b.client.Get(ctx, stateKey).Bytes()
		if errors.Is(err, redis.Nil) {
			return ErrRunOwnershipLost
		}
		if err != nil {
			return err
		}
		refData, err := b.client.Get(ctx, streamKey).Bytes()
		if errors.Is(err, redis.Nil) {
			return ErrRunOwnershipLost
		}
		if err != nil {
			return err
		}
		var ref StreamRef
		if err := unmarshalRuntimeJSON(refData, &ref); err != nil {
			return err
		}
		if ref.OwnerID != ownerID || ref.BotID != key.BotID || ref.SessionID != key.SessionID || ref.StreamID != streamID || ref.Generation != generation {
			return ErrRunOwnershipLost
		}
		snapshot, err := decodeRedisSnapshot(stateData)
		if err != nil {
			return err
		}
		if snapshot.CurrentRunView == nil {
			return ErrRunOwnershipLost
		}
		run := snapshot.CurrentRunView
		if run.StreamID != streamID || run.OwnerID != ownerID || run.Generation != generation || run.OwnerLeaseExpiresAt == nil {
			return ErrRunOwnershipLost
		}
		if !isActiveRunStatus(run.Status) {
			return nil
		}
		run.OwnerLeaseExpiresAt = &expiresAt
		snapshot.Queue = nonNilQueue(snapshot.Queue)
		nextStateData, err := marshalRuntimeJSON(snapshot)
		if err != nil {
			return err
		}
		result, err := renewRedisLeaseScript.Run(ctx, b.client, []string{stateKey, streamKey, b.runLeaseIndexKey()},
			stateData, refData, nextStateData, refData, expiresAt.UnixMilli(), b.stateTTL.Milliseconds(), redisRunLeaseMember(key),
		).Int64()
		if err != nil {
			return err
		}
		switch result {
		case 1:
			return nil
		case 0:
			if retryErr := waitRedisTransactionRetry(ctx, attempt); retryErr != nil {
				return retryErr
			}
			continue
		default:
			return ErrRunOwnershipLost
		}
	}
	return ErrBackendConflict
}

func (b *RedisBackend) ValidateRunOwnership(ctx context.Context, key Key, ref StreamRef) error {
	ref.BotID = strings.TrimSpace(ref.BotID)
	ref.SessionID = strings.TrimSpace(ref.SessionID)
	ref.StreamID = strings.TrimSpace(ref.StreamID)
	ref.OwnerID = strings.TrimSpace(ref.OwnerID)
	ref.Generation = strings.TrimSpace(ref.Generation)
	if ref.BotID != strings.TrimSpace(key.BotID) || ref.SessionID != strings.TrimSpace(key.SessionID) || ref.StreamID == "" || ref.OwnerID == "" || ref.Generation == "" {
		return ErrRunOwnershipLost
	}
	valid, err := validateRedisRunOwnershipScript.Run(ctx, b.client, []string{b.stateKey(key), b.streamKey(key, ref.StreamID)},
		ref.BotID, ref.SessionID, ref.StreamID, ref.OwnerID, ref.Generation,
	).Int64()
	if err != nil {
		return err
	}
	if valid != 1 {
		return ErrRunOwnershipLost
	}
	return nil
}

func (b *RedisBackend) Publish(ctx context.Context, event Event) error {
	data, err := marshalRuntimeJSON(event)
	if err != nil {
		return err
	}
	return b.client.Publish(ctx, b.sessionChannel(Key{BotID: event.BotID, SessionID: event.SessionID}), data).Err()
}

func (b *RedisBackend) Subscribe(ctx context.Context, key Key) (Subscription, error) {
	subCtx, cancel := context.WithCancel(ctx)
	pubsub := b.client.Subscribe(subCtx, b.sessionChannel(key))
	if _, err := pubsub.Receive(subCtx); err != nil {
		cancel()
		_ = pubsub.Close()
		return Subscription{}, err
	}
	subscriptionID, registered := b.registerSubscription(cancel)
	if !registered {
		cancel()
		_ = pubsub.Close()
		return Subscription{}, errRedisBackendClosed
	}
	ch := make(chan Event, 64)
	done := make(chan struct{})
	go func() {
		defer func() {
			_ = pubsub.Close()
			cancel()
			close(ch)
			b.unregisterSubscription(subscriptionID)
			close(done)
		}()
		for {
			msg, err := pubsub.ReceiveMessage(subCtx)
			if err != nil {
				if subCtx.Err() != nil {
					return
				}
				enqueueRuntimeEvent(ch, Event{
					Type:      EventRuntimeDropped,
					BotID:     key.BotID,
					SessionID: key.SessionID,
					Message:   "runtime pubsub receive interrupted",
				})
				if !waitRedisSubscriptionRetry(subCtx) {
					return
				}
				continue
			}
			var event Event
			if err := unmarshalRuntimeJSON([]byte(msg.Payload), &event); err != nil {
				enqueueRuntimeEvent(ch, Event{
					Type:      EventRuntimeDropped,
					BotID:     key.BotID,
					SessionID: key.SessionID,
					Message:   "runtime pubsub event is invalid",
				})
				continue
			}
			enqueueRuntimeEvent(ch, event)
		}
	}()
	var closeOnce sync.Once
	return Subscription{
		C: ch,
		Close: func() {
			closeOnce.Do(func() {
				cancel()
				_ = pubsub.Close()
			})
			<-done
		},
	}, nil
}

func (b *RedisBackend) LoadStreamRef(ctx context.Context, key Key, streamID string) (StreamRef, bool, error) {
	data, err := b.client.Get(ctx, b.streamKey(key, streamID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return StreamRef{}, false, nil
	}
	if err != nil {
		return StreamRef{}, false, err
	}
	var ref StreamRef
	if err := unmarshalRuntimeJSON(data, &ref); err != nil {
		return StreamRef{}, false, err
	}
	return ref, true, nil
}

func (b *RedisBackend) DeleteStreamRef(ctx context.Context, ref StreamRef) (bool, error) {
	ref.StreamID = strings.TrimSpace(ref.StreamID)
	ref.Generation = strings.TrimSpace(ref.Generation)
	if ref.StreamID == "" || ref.Generation == "" {
		return false, nil
	}
	data, err := marshalRuntimeJSON(ref)
	if err != nil {
		return false, err
	}
	key := Key{BotID: ref.BotID, SessionID: ref.SessionID}
	deleted, err := deleteRedisStreamRefScript.Run(ctx, b.client, []string{b.streamKey(key, ref.StreamID), b.runLeaseIndexKey(), b.stateKey(key)}, data, redisRunLeaseMember(key)).Int64()
	return deleted == 1, err
}

func (b *RedisBackend) ListExpiredRunKeys(ctx context.Context, limit int64) ([]Key, error) {
	if limit <= 0 {
		limit = expiredRunReaperBatchSize
	}
	members, err := listExpiredRedisRunsScript.Run(
		ctx,
		b.client,
		[]string{b.runLeaseIndexKey()},
		limit,
		b.keyPrefix+"state:",
		redisExpiredRunClaimTTL.Milliseconds(),
	).StringSlice()
	if err != nil {
		return nil, err
	}
	keys := make([]Key, 0, len(members))
	for _, member := range members {
		var key Key
		if err := unmarshalRuntimeJSON([]byte(member), &key); err != nil {
			return nil, fmt.Errorf("decode expired runtime key: %w", err)
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func (b *RedisBackend) PublishCommand(ctx context.Context, ownerID string, command Command) error {
	data, err := marshalRuntimeJSON(command)
	if err != nil {
		return err
	}
	count, err := b.client.Publish(ctx, b.commandChannel(ownerID), data).Result()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrCommandOwnerUnavailable
	}
	return nil
}

func (b *RedisBackend) SubscribeCommands(ctx context.Context, ownerID string) (CommandSubscription, error) {
	subCtx, cancel := context.WithCancel(ctx)
	pubsub := b.client.Subscribe(subCtx, b.commandChannel(ownerID))
	if _, err := pubsub.Receive(subCtx); err != nil {
		cancel()
		_ = pubsub.Close()
		return CommandSubscription{}, err
	}
	subscriptionID, registered := b.registerSubscription(cancel)
	if !registered {
		cancel()
		_ = pubsub.Close()
		return CommandSubscription{}, errRedisBackendClosed
	}
	ch := make(chan Command, 64)
	done := make(chan struct{})
	go func() {
		defer func() {
			_ = pubsub.Close()
			cancel()
			close(ch)
			b.unregisterSubscription(subscriptionID)
			close(done)
		}()
		for {
			msg, err := pubsub.ReceiveMessage(subCtx)
			if err != nil {
				if !waitRedisSubscriptionRetry(subCtx) {
					return
				}
				continue
			}
			var command Command
			if err := unmarshalRuntimeJSON([]byte(msg.Payload), &command); err != nil {
				continue
			}
			select {
			case ch <- command:
			case <-subCtx.Done():
				return
			}
		}
	}()
	var closeOnce sync.Once
	return CommandSubscription{
		C: ch,
		Close: func() {
			closeOnce.Do(func() {
				cancel()
				_ = pubsub.Close()
			})
			<-done
		},
	}, nil
}

func (b *RedisBackend) StoreCommandResult(ctx context.Context, result Command, ttl time.Duration) error {
	commandID := strings.TrimSpace(result.ID)
	if strings.TrimSpace(result.Type) != CommandResult || commandID == "" {
		return errors.New("command result type and id are required")
	}
	if ttl <= 0 {
		return errors.New("command result ttl must be positive")
	}
	data, err := marshalRuntimeJSON(result)
	if err != nil {
		return err
	}
	return b.client.SetNX(ctx, b.commandResultKey(commandID), data, ttl).Err()
}

func (b *RedisBackend) LoadCommandResult(ctx context.Context, commandID string) (Command, bool, error) {
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		return Command{}, false, nil
	}
	data, err := b.client.Get(ctx, b.commandResultKey(commandID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return Command{}, false, nil
	}
	if err != nil {
		return Command{}, false, err
	}
	var result Command
	if err := unmarshalRuntimeJSON(data, &result); err != nil {
		return Command{}, false, err
	}
	if strings.TrimSpace(result.Type) != CommandResult || strings.TrimSpace(result.ID) != commandID {
		return Command{}, false, errors.New("stored runtime command result is invalid")
	}
	return result, true, nil
}

func (b *RedisBackend) Close() error {
	b.closeOnce.Do(func() {
		b.subscriptionsMu.Lock()
		b.closed = true
		cancels := make([]context.CancelFunc, 0, len(b.subscriptions))
		for _, cancel := range b.subscriptions {
			cancels = append(cancels, cancel)
		}
		b.subscriptionsMu.Unlock()

		for _, cancel := range cancels {
			cancel()
		}
		b.closeErr = b.client.Close()
		b.subscriptionsWG.Wait()
		close(b.closeDone)
	})
	<-b.closeDone
	return b.closeErr
}

func loadRedisSnapshot(ctx context.Context, tx *redis.Tx, stateKey, contentKey, revisionKey string) (Snapshot, bool, error) {
	data, err := tx.Get(ctx, stateKey).Bytes()
	if errors.Is(err, redis.Nil) {
		return Snapshot{}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, err
	}
	contents, err := tx.HGetAll(ctx, contentKey).Result()
	if err != nil {
		return Snapshot{}, false, err
	}
	revision, err := tx.HGetAll(ctx, revisionKey).Result()
	if err != nil {
		return Snapshot{}, false, err
	}
	snapshot, err := decodeRedisSnapshot(data)
	if err != nil {
		return Snapshot{}, false, err
	}
	hydrateRedisSnapshot(&snapshot, contents, revision)
	return snapshot, true, nil
}

type preparedRedisSnapshot struct {
	stateData []byte
	contents  map[string]string
	revision  map[string]string
}

func prepareRedisSnapshot(snapshot Snapshot) (preparedRedisSnapshot, error) {
	persisted := snapshot
	contents := make(map[string]string)
	if snapshot.CurrentRunView != nil {
		run := *snapshot.CurrentRunView
		run.Messages = append([]conversation.UIMessage(nil), snapshot.CurrentRunView.Messages...)
		for index := range run.Messages {
			message := &run.Messages[index]
			if message.Type != conversation.UIMessageText && message.Type != conversation.UIMessageReasoning {
				continue
			}
			contents[redisMessageContentField(run.Generation, *message)] = message.Content
			message.Content = ""
		}
		persisted.CurrentRunView = &run
	}
	stateData, err := marshalRuntimeJSON(persisted)
	if err != nil {
		return preparedRedisSnapshot{}, err
	}
	revision := map[string]string{
		"epoch":                   snapshot.Epoch,
		"seq":                     strconv.FormatInt(snapshot.Seq, 10),
		"updated_at_seconds":      strconv.FormatInt(snapshot.UpdatedAt.Unix(), 10),
		"updated_at_microseconds": strconv.FormatInt(int64(snapshot.UpdatedAt.Nanosecond())/int64(time.Microsecond), 10),
	}
	return preparedRedisSnapshot{stateData: stateData, contents: contents, revision: revision}, nil
}

func writeRedisSnapshot(ctx context.Context, pipe redis.Pipeliner, stateKey, contentKey, revisionKey string, prepared preparedRedisSnapshot, ttl time.Duration) {
	pipe.Set(ctx, stateKey, prepared.stateData, ttl)
	pipe.Del(ctx, contentKey)
	if len(prepared.contents) > 0 {
		pipe.HSet(ctx, contentKey, prepared.contents)
		pipe.PExpire(ctx, contentKey, ttl)
	}
	pipe.Del(ctx, revisionKey)
	pipe.HSet(ctx, revisionKey, prepared.revision)
	pipe.PExpire(ctx, revisionKey, ttl)
}

func redisMessageContentField(generation string, message conversation.UIMessage) string {
	return strings.TrimSpace(generation) + ":" + strconv.Itoa(message.ID) + ":" + string(message.Type)
}

func decodeRedisSnapshot(data []byte) (Snapshot, error) {
	var snapshot Snapshot
	if err := unmarshalRuntimeJSON(data, &snapshot); err != nil {
		return Snapshot{}, err
	}
	snapshot.Queue = nonNilQueue(snapshot.Queue)
	return snapshot, nil
}

func hydrateRedisSnapshot(snapshot *Snapshot, contents, revision map[string]string) {
	if snapshot == nil {
		return
	}
	if seq, err := strconv.ParseInt(revision["seq"], 10, 64); err == nil {
		snapshot.Seq = seq
	}
	if epoch := strings.TrimSpace(revision["epoch"]); epoch != "" {
		snapshot.Epoch = epoch
	}
	seconds, secondsErr := strconv.ParseInt(revision["updated_at_seconds"], 10, 64)
	microseconds, microsErr := strconv.ParseInt(revision["updated_at_microseconds"], 10, 64)
	if secondsErr == nil && microsErr == nil {
		updatedAt := time.Unix(seconds, microseconds*int64(time.Microsecond)).UTC()
		snapshot.UpdatedAt = updatedAt
		if snapshot.CurrentRunView != nil {
			snapshot.CurrentRunView.UpdatedAt = updatedAt
		}
	}
	if snapshot.CurrentRunView == nil {
		return
	}
	for index := range snapshot.CurrentRunView.Messages {
		message := &snapshot.CurrentRunView.Messages[index]
		if content, ok := contents[redisMessageContentField(snapshot.CurrentRunView.Generation, *message)]; ok {
			message.Content = content
		}
	}
}

func waitRedisSubscriptionRetry(ctx context.Context) bool {
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func waitRedisTransactionRetry(ctx context.Context, attempt int) error {
	delay := 100 * time.Microsecond * time.Duration(1<<min(attempt, 6))
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func loadRedisStreamRef(ctx context.Context, tx *redis.Tx, key string) (StreamRef, bool, error) {
	data, err := tx.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return StreamRef{}, false, nil
	}
	if err != nil {
		return StreamRef{}, false, err
	}
	var ref StreamRef
	if err := unmarshalRuntimeJSON(data, &ref); err != nil {
		return StreamRef{}, false, err
	}
	return ref, true, nil
}

func redisScriptInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case int64:
		return typed, nil
	case string:
		return strconv.ParseInt(typed, 10, 64)
	case []byte:
		return strconv.ParseInt(string(typed), 10, 64)
	default:
		return strconv.ParseInt(fmt.Sprint(value), 10, 64)
	}
}

func (b *RedisBackend) stateKey(key Key) string {
	return b.keyPrefix + "state:" + key.String()
}

func (b *RedisBackend) contentKey(key Key) string {
	return b.keyPrefix + "content:" + key.String()
}

func (b *RedisBackend) revisionKey(key Key) string {
	return b.keyPrefix + "revision:" + key.String()
}

func (b *RedisBackend) streamKey(key Key, streamID string) string {
	return b.keyPrefix + "stream:" + key.String() + ":" + strings.TrimSpace(streamID)
}

func (b *RedisBackend) sessionChannel(key Key) string {
	return b.keyPrefix + "events:" + key.String()
}

func (b *RedisBackend) commandChannel(ownerID string) string {
	return b.keyPrefix + "commands:" + strings.TrimSpace(ownerID)
}

func (b *RedisBackend) commandResultKey(commandID string) string {
	return b.keyPrefix + "command_result:" + strings.TrimSpace(commandID)
}

func (b *RedisBackend) runLeaseIndexKey() string {
	return b.keyPrefix + "run_leases"
}

func redisRunLeaseMember(key Key) string {
	data, _ := marshalRuntimeJSON(Key{BotID: strings.TrimSpace(key.BotID), SessionID: strings.TrimSpace(key.SessionID)})
	return string(data)
}
