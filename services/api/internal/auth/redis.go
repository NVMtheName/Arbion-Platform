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
	return s.client.Del(ctx, s.prefix+"session:"+tokenKey(token)).Err()
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
