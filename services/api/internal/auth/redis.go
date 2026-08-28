package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	client *redis.Client
	prefix string
	now    func() time.Time
}

var weightedRateLimitScript = redis.NewScript(`
local current = tonumber(redis.call("GET", KEYS[1]) or "0")
local cost = tonumber(ARGV[1])
local limit = tonumber(ARGV[2])
if current > 0 and redis.call("PTTL", KEYS[1]) < 0 then
  return redis.error_reply("rate limit key is missing expiry")
end
if current + cost > limit then
  return 0
end
local next = redis.call("INCRBY", KEYS[1], cost)
if next == cost then
  redis.call("PEXPIRE", KEYS[1], ARGV[3])
end
return 1
`)

var revokeOtherSessionsScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[2]) == 0 then
  return -1
end
local members = redis.call("SMEMBERS", KEYS[1])
local revoked = 0
for _, key in ipairs(members) do
  if key ~= KEYS[2] then
    local raw = redis.call("GET", key)
    local owner_prefix = '{"user_id":"' .. ARGV[1] .. '"'
    if string.sub(key, 1, string.len(ARGV[2])) == ARGV[2]
      and raw
      and string.sub(raw, 1, string.len(owner_prefix)) == owner_prefix then
      revoked = revoked + redis.call("DEL", key)
    end
    redis.call("SREM", KEYS[1], key)
  end
end
redis.call("SADD", KEYS[1], KEYS[2])
local current_ttl = redis.call("PTTL", KEYS[2])
local set_ttl = redis.call("PTTL", KEYS[1])
if current_ttl > 0 and (set_ttl < 0 or set_ttl < current_ttl) then
  redis.call("PEXPIRE", KEYS[1], current_ttl)
end
return revoked
`)

func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{client: client, prefix: "arbion:", now: time.Now}
}
func tokenKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func (s *RedisStore) Create(ctx context.Context, userID string, ttl time.Duration) (string, Session, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", Session{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	now := s.now().UTC()
	sess := Session{UserID: userID, CreatedAt: now, LastActivityAt: now, ExpiresAt: now.Add(ttl)}
	raw, _ := json.Marshal(sess)
	key := s.prefix + "session:" + tokenKey(token)
	pipe := s.client.TxPipeline()
	pipe.Set(ctx, key, raw, ttl)
	pipe.SAdd(ctx, s.prefix+"user_sessions:"+userID, key)
	pipe.Expire(ctx, s.prefix+"user_sessions:"+userID, ttl)
	_, err := pipe.Exec(ctx)
	return token, sess, err
}
func (s *RedisStore) Get(ctx context.Context, token string) (Session, error) {
	if token == "" {
		return Session{}, ErrUnauthenticated
	}
	raw, err := s.client.Get(ctx, s.prefix+"session:"+tokenKey(token)).Bytes()
	if err != nil {
		return Session{}, ErrUnauthenticated
	}
	var sess Session
	if json.Unmarshal(raw, &sess) != nil || !s.now().Before(sess.ExpiresAt) {
		return Session{}, ErrUnauthenticated
	}
	return sess, nil
}
func (s *RedisStore) Delete(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	key := s.prefix + "session:" + tokenKey(token)
	sess, err := s.Get(ctx, token)
	if err != nil {
		return s.client.Del(ctx, key).Err()
	}
	pipe := s.client.TxPipeline()
	pipe.Del(ctx, key)
	pipe.SRem(ctx, s.prefix+"user_sessions:"+sess.UserID, key)
	_, err = pipe.Exec(ctx)
	return err
}
func (s *RedisStore) RevokeUser(ctx context.Context, userID string) error {
	sessionSet := s.prefix + "user_sessions:" + userID
	challengeSet := s.prefix + "user_mfa_challenges:" + userID
	keys := []string{sessionSet, challengeSet}
	for _, set := range []string{sessionSet, challengeSet} {
		members, err := s.client.SMembers(ctx, set).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		keys = append(keys, members...)
	}
	return s.client.Del(ctx, keys...).Err()
}

func (s *RedisStore) SessionInventory(ctx context.Context, userID, currentToken string, limit int) (SessionInventory, error) {
	if userID == "" || currentToken == "" {
		return SessionInventory{}, ErrUnauthenticated
	}
	if limit < 1 || limit > 100 {
		return SessionInventory{}, ErrSessionInventoryUnavailable
	}
	current, err := s.Get(ctx, currentToken)
	if err != nil || current.UserID != userID || current.CreatedAt.IsZero() || current.ExpiresAt.IsZero() {
		return SessionInventory{}, ErrUnauthenticated
	}

	setKey := s.prefix + "user_sessions:" + userID
	currentKey := s.prefix + "session:" + tokenKey(currentToken)
	members := make([]string, 0, limit)
	seen := make(map[string]struct{}, limit)
	var cursor uint64
	for {
		var batch []string
		batch, cursor, err = s.client.SScan(ctx, setKey, cursor, "", 64).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return SessionInventory{}, ErrSessionInventoryUnavailable
		}
		for _, member := range batch {
			if _, exists := seen[member]; exists {
				continue
			}
			seen[member] = struct{}{}
			members = append(members, member)
			if len(members) > limit*5 {
				return SessionInventory{}, ErrSessionInventoryUnavailable
			}
		}
		if cursor == 0 {
			break
		}
	}
	if _, exists := seen[currentKey]; !exists {
		if err = s.client.SAdd(ctx, setKey, currentKey).Err(); err != nil {
			return SessionInventory{}, ErrSessionInventoryUnavailable
		}
		if ttl, ttlErr := s.client.PTTL(ctx, setKey).Result(); ttlErr == nil && ttl < 0 {
			remaining := current.ExpiresAt.Sub(s.now().UTC())
			if remaining > 0 {
				_ = s.client.PExpire(ctx, setKey, remaining).Err()
			}
		}
		members = append(members, currentKey)
	}

	values, err := s.client.MGet(ctx, members...).Result()
	if err != nil {
		return SessionInventory{}, ErrSessionInventoryUnavailable
	}
	now := s.now().UTC()
	active := 0
	currentFound := false
	stale := make([]any, 0)
	for index, value := range values {
		raw, ok := value.(string)
		if !ok {
			stale = append(stale, members[index])
			continue
		}
		var session Session
		if json.Unmarshal([]byte(raw), &session) != nil || session.UserID != userID || session.CreatedAt.IsZero() || session.ExpiresAt.IsZero() || !now.Before(session.ExpiresAt) {
			stale = append(stale, members[index])
			continue
		}
		active++
		currentFound = currentFound || members[index] == currentKey
	}
	if len(stale) > 0 {
		_ = s.client.SRem(ctx, setKey, stale...).Err()
	}
	if !currentFound {
		return SessionInventory{}, ErrUnauthenticated
	}
	if active > limit {
		return SessionInventory{}, ErrSessionInventoryUnavailable
	}
	return SessionInventory{
		ActiveCount: active,
		OtherCount:  active - 1,
		Current:     SessionWindow{CreatedAt: current.CreatedAt, ExpiresAt: current.ExpiresAt},
	}, nil
}

func (s *RedisStore) RevokeUserExcept(ctx context.Context, userID, currentToken string) (int, error) {
	if userID == "" || currentToken == "" {
		return 0, ErrUnauthenticated
	}
	current, err := s.Get(ctx, currentToken)
	if err != nil || current.UserID != userID {
		return 0, ErrUnauthenticated
	}
	setKey := s.prefix + "user_sessions:" + userID
	currentKey := s.prefix + "session:" + tokenKey(currentToken)
	revoked, err := revokeOtherSessionsScript.Run(ctx, s.client, []string{setKey, currentKey}, userID, s.prefix+"session:").Int()
	if err != nil {
		return 0, err
	}
	if revoked < 0 {
		return 0, ErrUnauthenticated
	}
	return revoked, nil
}

func (s *RedisStore) CreateMFAChallenge(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	if userID == "" || ttl <= 0 {
		return "", ErrMFAUnavailable
	}
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(rawToken)
	challenge := MFAChallenge{UserID: userID, ExpiresAt: s.now().UTC().Add(ttl)}
	rawChallenge, err := json.Marshal(challenge)
	if err != nil {
		return "", err
	}
	key := s.prefix + "mfa_challenge:" + tokenKey(token)
	set := s.prefix + "user_mfa_challenges:" + userID
	pipe := s.client.TxPipeline()
	pipe.Set(ctx, key, rawChallenge, ttl)
	pipe.SAdd(ctx, set, key)
	pipe.Expire(ctx, set, ttl)
	_, err = pipe.Exec(ctx)
	return token, err
}

func (s *RedisStore) GetMFAChallenge(ctx context.Context, token string) (MFAChallenge, error) {
	return s.readMFAChallenge(ctx, token, false)
}

func (s *RedisStore) ConsumeMFAChallenge(ctx context.Context, token string) (MFAChallenge, error) {
	return s.readMFAChallenge(ctx, token, true)
}

func (s *RedisStore) readMFAChallenge(ctx context.Context, token string, consume bool) (MFAChallenge, error) {
	if token == "" {
		return MFAChallenge{}, ErrInvalidMFAChallenge
	}
	key := s.prefix + "mfa_challenge:" + tokenKey(token)
	var raw []byte
	var err error
	if consume {
		raw, err = s.client.GetDel(ctx, key).Bytes()
	} else {
		raw, err = s.client.Get(ctx, key).Bytes()
	}
	if err != nil {
		return MFAChallenge{}, ErrInvalidMFAChallenge
	}
	var challenge MFAChallenge
	if json.Unmarshal(raw, &challenge) != nil || challenge.UserID == "" || !s.now().Before(challenge.ExpiresAt) {
		return MFAChallenge{}, ErrInvalidMFAChallenge
	}
	if consume {
		_ = s.client.SRem(ctx, s.prefix+"user_mfa_challenges:"+challenge.UserID, key).Err()
	}
	return challenge, nil
}

func (s *RedisStore) DeleteMFAChallenge(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	if _, err := s.readMFAChallenge(ctx, token, true); err != nil {
		return s.client.Del(ctx, s.prefix+"mfa_challenge:"+tokenKey(token)).Err()
	}
	return nil
}

func (s *RedisStore) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error) {
	k := s.prefix + "rate:" + tokenKey(key)
	n, err := s.client.Incr(ctx, k).Result()
	if err != nil {
		return false, err
	}
	if n == 1 {
		if err := s.client.Expire(ctx, k, window).Err(); err != nil {
			return false, err
		}
	}
	return n <= int64(limit), nil
}

func (s *RedisStore) AllowWeighted(ctx context.Context, key string, cost, limit int, window time.Duration) (bool, error) {
	if cost <= 0 || limit <= 0 || window <= 0 {
		return false, errors.New("invalid weighted rate limit")
	}
	k := s.prefix + "rate:" + tokenKey(key)
	allowed, err := weightedRateLimitScript.Run(ctx, s.client, []string{k}, cost, limit, window.Milliseconds()).Int()
	return allowed == 1, err
}
