package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const MinPasswordBytes = 12
const MaxPasswordBytes = 1024

var ErrInvalidPassword = errors.New("password must be between 12 and 1024 bytes")

type PasswordHasher struct {
	memory     uint32
	time       uint32
	threads    uint8
	saltLength uint32
	keyLength  uint32
}

func NewPasswordHasher() PasswordHasher {
	return PasswordHasher{memory: 64 * 1024, time: 3, threads: 2, saltLength: 16, keyLength: 32}
}

func (h PasswordHasher) Hash(password string) (string, error) {
	if len(password) < MinPasswordBytes || len(password) > MaxPasswordBytes {
		return "", ErrInvalidPassword
	}
	salt := make([]byte, h.saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, h.time, h.memory, h.threads, h.keyLength)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s", h.memory, h.time, h.threads, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func (h PasswordHasher) Verify(password, encoded string) bool {
	if len(password) > MaxPasswordBytes {
		return false
	}
	var memory, iterations uint32
	var threads uint8
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); err != nil {
		return false
	}
	if memory > 256*1024 || iterations > 10 || threads > 16 || memory == 0 || iterations == 0 || threads == 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) < 16 || len(want) > 64 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iterations, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
