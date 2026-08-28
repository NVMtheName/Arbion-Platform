package credential

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

var ErrNotFound = errors.New("credential not found")

type Class string

const (
	AI        Class = "ai"
	Financial Class = "financial"
)

type Locator struct {
	ConnectionID string
	UserID       string
	Class        Class
}

func (l Locator) aad() []byte {
	return []byte(string(l.Class) + "\x00" + l.UserID + "\x00" + l.ConnectionID)
}

type Vault interface {
	Store(context.Context, Locator, []byte) error
	Retrieve(context.Context, Locator) ([]byte, error)
	Replace(context.Context, Locator, []byte) error
	Delete(context.Context, Locator) error
}
type StagedVault interface {
	Vault
	Stage(context.Context, Locator, []byte) (string, error)
	DiscardStaged(context.Context, Locator, string) error
}
type BlobStore interface {
	Put(context.Context, Locator, []byte, bool) error
	Get(context.Context, Locator) ([]byte, error)
	Delete(context.Context, Locator) error
}
type StagedBlobStore interface {
	BlobStore
	PutStaged(context.Context, Locator, []byte, string) error
	DeleteStaged(context.Context, Locator, string) error
}
type EncryptedVault struct {
	aead  cipher.AEAD
	store BlobStore
}

func NewEncryptedVault(key []byte, store BlobStore) (*EncryptedVault, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create credential cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create credential AEAD: %w", err)
	}
	return &EncryptedVault{aead: aead, store: store}, nil
}
func (v *EncryptedVault) seal(l Locator, plaintext []byte) ([]byte, error) {
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate credential nonce: %w", err)
	}
	return v.aead.Seal(nonce, nonce, plaintext, l.aad()), nil
}
func (v *EncryptedVault) Store(ctx context.Context, l Locator, p []byte) error {
	c, e := v.seal(l, p)
	if e != nil {
		return e
	}
	return v.store.Put(ctx, l, c, true)
}
func (v *EncryptedVault) Replace(ctx context.Context, l Locator, p []byte) error {
	c, e := v.seal(l, p)
	if e != nil {
		return e
	}
	return v.store.Put(ctx, l, c, false)
}
func (v *EncryptedVault) Stage(ctx context.Context, l Locator, p []byte) (string, error) {
	store, ok := v.store.(StagedBlobStore)
	if !ok {
		return "", errors.New("credential staging is unavailable")
	}
	tokenBytes := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, tokenBytes); err != nil {
		return "", fmt.Errorf("generate credential staging token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)
	ciphertext, err := v.seal(l, p)
	if err != nil {
		return "", err
	}
	if err = store.PutStaged(ctx, l, ciphertext, token); err != nil {
		return "", err
	}
	return token, nil
}
func (v *EncryptedVault) DiscardStaged(ctx context.Context, l Locator, token string) error {
	store, ok := v.store.(StagedBlobStore)
	if !ok {
		return errors.New("credential staging is unavailable")
	}
	return store.DeleteStaged(ctx, l, token)
}
func (v *EncryptedVault) Retrieve(ctx context.Context, l Locator) ([]byte, error) {
	c, e := v.store.Get(ctx, l)
	if e != nil {
		return nil, e
	}
	n := v.aead.NonceSize()
	if len(c) < n {
		return nil, errors.New("credential ciphertext is invalid")
	}
	p, e := v.aead.Open(nil, c[:n], c[n:], l.aad())
	if e != nil {
		return nil, errors.New("credential authentication failed")
	}
	return p, nil
}
func (v *EncryptedVault) Delete(ctx context.Context, l Locator) error { return v.store.Delete(ctx, l) }
