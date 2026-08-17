package responsecache

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

const (
	redisSchemaVersion = "1"
	defaultRedisPrefix = "omnillm"
)

var (
	saveScript = redis.NewScript(`
redis.call('DEL', KEYS[1])
redis.call('HSET', KEYS[1],
  'response_data', ARGV[1],
  'model_id', ARGV[2],
  'created_at_ms', ARGV[3],
  'hit_count', 0,
  'last_hit_at_ms', '',
  'schema_version', ARGV[4])
redis.call('PEXPIRE', KEYS[1], ARGV[5])
return 1
`)
	getScript = redis.NewScript(`
local values = redis.call('HMGET', KEYS[1],
  'response_data', 'model_id', 'created_at_ms', 'schema_version')
if not values[1] and not values[2] and not values[3] and not values[4] then
  return nil
end
if not values[1] or not values[2] or not values[3] or values[4] ~= ARGV[1] then
  return {'__omnillm_malformed__'}
end
local hits = redis.call('HINCRBY', KEYS[1], 'hit_count', 1)
redis.call('HSET', KEYS[1], 'last_hit_at_ms', ARGV[2])
return {values[1], values[2], values[3], tostring(hits), ARGV[2]}
`)
)

// RedisStoreConfig controls Redis key isolation and bounded failure behavior.
type RedisStoreConfig struct {
	Prefix           string
	CommandTimeout   time.Duration
	CircuitCooldown  time.Duration
	RecoveryInterval time.Duration
	ScanCount        int64
	BatchSize        int
}

func (c RedisStoreConfig) normalized() (RedisStoreConfig, error) {
	if c.Prefix == "" {
		c.Prefix = defaultRedisPrefix
	}
	if err := validatePrefix(c.Prefix); err != nil {
		return RedisStoreConfig{}, err
	}
	if c.CommandTimeout <= 0 {
		c.CommandTimeout = 250 * time.Millisecond
	}
	if c.CircuitCooldown <= 0 {
		c.CircuitCooldown = time.Second
	}
	if c.RecoveryInterval <= 0 {
		c.RecoveryInterval = c.CircuitCooldown
	}
	if c.ScanCount <= 0 {
		c.ScanCount = 128
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 128
	}
	return c, nil
}

// RedisStore stores exact responses in versioned, namespaced Redis hashes.
type RedisStore struct {
	client redis.UniversalClient
	config RedisStoreConfig
	entry  string

	clearMu sync.RWMutex
	stateMu sync.Mutex
	state   circuitState

	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	closeErr  error
}

type circuitState struct {
	available bool
	probing   bool
	nextProbe time.Time
	closed    bool
}

// NewRedisStore validates config and starts bounded background recovery. The
// store assumes ownership of client and closes it from Close.
func NewRedisStore(client redis.UniversalClient, config RedisStoreConfig) (*RedisStore, error) {
	if client == nil {
		return nil, errors.New("response cache Redis client is nil")
	}
	normalized, err := config.normalized()
	if err != nil {
		return nil, err
	}
	store := &RedisStore{
		client: client,
		config: normalized,
		entry:  normalized.Prefix + ":response-cache:v1:entry:",
		state:  circuitState{available: true},
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	go store.recoverLoop()
	return store, nil
}

// Ping checks Redis immediately and updates availability. It is suitable for a
// bounded startup check and deliberately bypasses an open circuit.
func (s *RedisStore) Ping(ctx context.Context) error {
	if err := s.closedError(); err != nil {
		return err
	}
	commandCtx, cancel := s.commandContext(ctx)
	defer cancel()
	err := s.client.Ping(commandCtx).Err()
	s.finishCommand(ctx, err)
	return err
}

// Available reports the most recently observed backend state.
func (s *RedisStore) Available() bool {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.state.available && !s.state.closed
}

// Get atomically reads a valid entry and increments its hit metadata without
// changing its native expiration.
func (s *RedisStore) Get(ctx context.Context, key string) (*Record, error) {
	redisKey, err := s.redisKey(key)
	if err != nil {
		return nil, err
	}
	if err := s.beginCommand(); err != nil {
		return nil, err
	}
	commandCtx, cancel := s.commandContext(ctx)
	defer cancel()
	now := time.Now().UTC()
	result, err := getScript.Run(commandCtx, s.client, []string{redisKey}, redisSchemaVersion, now.UnixMilli()).Result()
	if errors.Is(err, redis.Nil) {
		s.finishCommand(ctx, nil)
		return nil, nil
	}
	if err != nil {
		s.finishCommand(ctx, err)
		return nil, err
	}
	s.finishCommand(ctx, nil)

	values, ok := result.([]interface{})
	if !ok || len(values) == 0 || stringValue(values[0]) == "__omnillm_malformed__" {
		s.deleteCorrupt(ctx, redisKey)
		return nil, nil
	}
	if len(values) != 5 {
		s.deleteCorrupt(ctx, redisKey)
		return nil, nil
	}
	responseData := stringValue(values[0])
	createdAtMS, createdErr := strconv.ParseInt(stringValue(values[2]), 10, 64)
	hitCount, hitErr := strconv.ParseInt(stringValue(values[3]), 10, 64)
	lastHitMS, lastHitErr := strconv.ParseInt(stringValue(values[4]), 10, 64)
	if responseData == "" || !json.Valid([]byte(responseData)) || createdErr != nil || hitErr != nil || lastHitErr != nil {
		s.deleteCorrupt(ctx, redisKey)
		return nil, nil
	}
	lastHit := time.UnixMilli(lastHitMS).UTC()
	return &Record{
		Key:          key,
		ModelID:      stringValue(values[1]),
		ResponseData: responseData,
		HitCount:     hitCount,
		CreatedAt:    time.UnixMilli(createdAtMS).UTC(),
		LastHitAt:    &lastHit,
	}, nil
}

// Save atomically replaces an entry, resets its hit metadata, and assigns its
// creation-age TTL. Existing entries keep their prior TTL until refreshed.
func (s *RedisStore) Save(ctx context.Context, key, modelID, responseData string, ttl time.Duration) error {
	if ttl <= 0 {
		return errors.New("response cache TTL must be positive")
	}
	if responseData == "" || !json.Valid([]byte(responseData)) {
		return errors.New("response cache data must be valid JSON")
	}
	redisKey, err := s.redisKey(key)
	if err != nil {
		return err
	}
	if err := s.beginCommand(); err != nil {
		return err
	}
	s.clearMu.RLock()
	defer s.clearMu.RUnlock()
	commandCtx, cancel := s.commandContext(ctx)
	defer cancel()
	ttlMillis := max(ttl.Milliseconds(), int64(1))
	err = saveScript.Run(commandCtx, s.client, []string{redisKey}, responseData, modelID, time.Now().UTC().UnixMilli(), redisSchemaVersion, ttlMillis).Err()
	s.finishCommand(ctx, err)
	return err
}

// Stats scans only this store's versioned namespace and pipelines metadata
// reads in bounded batches.
func (s *RedisStore) Stats(ctx context.Context) (Stats, error) {
	if err := s.beginCommand(); err != nil {
		return Stats{}, err
	}
	commandCtx, cancel := s.commandContext(ctx)
	defer cancel()
	var stats Stats
	var cursor uint64
	for {
		keys, next, err := s.client.Scan(commandCtx, cursor, s.entry+"*", s.config.ScanCount).Result()
		if err != nil {
			s.finishCommand(ctx, err)
			return Stats{}, err
		}
		for start := 0; start < len(keys); start += s.config.BatchSize {
			end := min(start+s.config.BatchSize, len(keys))
			commands := make([]*redis.StringCmd, 0, end-start)
			_, err = s.client.Pipelined(commandCtx, func(pipe redis.Pipeliner) error {
				for _, key := range keys[start:end] {
					commands = append(commands, pipe.HGet(commandCtx, key, "hit_count"))
				}
				return nil
			})
			if err != nil && !errors.Is(err, redis.Nil) {
				s.finishCommand(ctx, err)
				return Stats{}, err
			}
			stats.Entries += int64(len(commands))
			for _, command := range commands {
				hits, parseErr := command.Int64()
				if parseErr == nil {
					stats.TotalHits += hits
				}
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	s.finishCommand(ctx, nil)
	return stats, nil
}

// Clear removes only this store's versioned namespace. Local saves cannot
// complete during the operation.
func (s *RedisStore) Clear(ctx context.Context) (int64, error) {
	if err := s.beginCommand(); err != nil {
		return 0, err
	}
	s.clearMu.Lock()
	defer s.clearMu.Unlock()
	commandCtx, cancel := s.commandContext(ctx)
	defer cancel()
	var removed int64
	for {
		var cursor uint64
		var discovered int
		for {
			keys, next, err := s.client.Scan(commandCtx, cursor, s.entry+"*", s.config.ScanCount).Result()
			if err != nil {
				s.finishCommand(ctx, err)
				return removed, err
			}
			discovered += len(keys)
			for start := 0; start < len(keys); start += s.config.BatchSize {
				end := min(start+s.config.BatchSize, len(keys))
				count, deleteErr := s.client.Unlink(commandCtx, keys[start:end]...).Result()
				if deleteErr != nil && unlinkUnsupported(deleteErr) {
					count, deleteErr = s.client.Del(commandCtx, keys[start:end]...).Result()
				}
				if deleteErr != nil {
					s.finishCommand(ctx, deleteErr)
					return removed, deleteErr
				}
				removed += count
			}
			cursor = next
			if cursor == 0 {
				break
			}
		}
		// Deleting while scanning may move Redis hash-table buckets past the
		// cursor. Repeat complete passes until one discovers no keys.
		if discovered == 0 {
			break
		}
	}
	s.finishCommand(ctx, nil)
	return removed, nil
}

// Close stops recovery and closes the owned Redis client exactly once.
func (s *RedisStore) Close() error {
	s.closeOnce.Do(func() {
		s.stateMu.Lock()
		s.state.closed = true
		s.state.available = false
		s.stateMu.Unlock()
		close(s.stop)
		<-s.done
		s.closeErr = s.client.Close()
	})
	return s.closeErr
}

func (s *RedisStore) beginCommand() error {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.state.closed {
		return ErrStoreClosed
	}
	if s.state.available {
		return nil
	}
	if s.state.probing || time.Now().Before(s.state.nextProbe) {
		return ErrStoreUnavailable
	}
	s.state.probing = true
	return nil
}

func (s *RedisStore) finishCommand(caller context.Context, err error) {
	// A deadline imposed by the caller is not evidence that Redis is unhealthy.
	if err != nil && caller.Err() != nil {
		s.stateMu.Lock()
		s.state.probing = false
		s.stateMu.Unlock()
		return
	}
	available := err == nil
	s.stateMu.Lock()
	previous := s.state.available
	s.state.available = available
	s.state.probing = false
	if available {
		s.state.nextProbe = time.Time{}
	} else {
		s.state.nextProbe = time.Now().Add(s.config.CircuitCooldown)
	}
	closed := s.state.closed
	s.stateMu.Unlock()
	if !closed && previous != available {
		log.Warn().Bool("available", available).Msg("Response cache Redis availability changed")
	}
}

func (s *RedisStore) recoverLoop() {
	defer close(s.done)
	ticker := time.NewTicker(s.config.RecoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if s.Available() {
				continue
			}
			if err := s.beginCommand(); err != nil {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), s.config.CommandTimeout)
			err := s.client.Ping(ctx).Err()
			cancel()
			s.finishCommand(context.Background(), err)
		case <-s.stop:
			return
		}
	}
}

func (s *RedisStore) commandContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, s.config.CommandTimeout)
}

func (s *RedisStore) closedError() error {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.state.closed {
		return ErrStoreClosed
	}
	return nil
}

func (s *RedisStore) redisKey(key string) (string, error) {
	decoded, err := hex.DecodeString(key)
	if err != nil || len(decoded) != sha256Size || strings.ToLower(key) != key {
		return "", errors.New("response cache key must be a lowercase SHA-256 digest")
	}
	return s.entry + key, nil
}

const sha256Size = 32

func (s *RedisStore) deleteCorrupt(ctx context.Context, key string) {
	commandCtx, cancel := s.commandContext(ctx)
	defer cancel()
	_ = s.client.Unlink(commandCtx, key).Err()
}

func validatePrefix(prefix string) error {
	if len(prefix) > 128 {
		return errors.New("response cache Redis prefix is too long")
	}
	for _, char := range prefix {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return fmt.Errorf("response cache Redis prefix contains invalid character %q", char)
	}
	return nil
}

func stringValue(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(value)
	}
}

func unlinkUnsupported(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unknown command") && strings.Contains(message, "unlink")
}
