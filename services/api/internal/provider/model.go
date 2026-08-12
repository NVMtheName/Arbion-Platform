package provider

import (
	"encoding/json"
	"time"
)

type Category string

const (
	CategoryAI        Category = "ai"
	CategoryFinancial Category = "financial"
)

// Connection is the safe application/API model. Encrypted material is deliberately absent.
type Connection struct {
	ID                 string         `json:"id"`
	UserID             string         `json:"user_id"`
	Category           Category       `json:"provider_category"`
	ProviderName       string         `json:"provider_name"`
	DisplayName        string         `json:"display_name"`
	Status             string         `json:"status"`
	Scopes             []string       `json:"scopes"`
	CredentialMetadata map[string]any `json:"credential_metadata"`
	TokenExpiresAt     *time.Time     `json:"token_expires_at,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	LastVerifiedAt     *time.Time     `json:"last_verified_at,omitempty"`
}

func (c Connection) MarshalJSON() ([]byte, error) { type safe Connection; return json.Marshal(safe(c)) }
