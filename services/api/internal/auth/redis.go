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
	set := s.prefix + "user_sessions:" + userID
	keys, err := s.client.SMembers(ctx, set).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	keys = append(keys, set)
	return s.client.Del(ctx, keys...).Err()
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
