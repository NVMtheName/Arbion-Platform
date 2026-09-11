package etrade

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/arbion/platform/services/api/internal/financial"
)

// AccountInventory carries environment provenance even when empty. Sandbox
// inventory is canned provider test data, never a real portfolio observation.
// The future service must not persist it into a production financial account.
type AccountInventory struct {
	Provider    string                       `json:"provider"`
	Environment Environment                  `json:"environment"`
	Synthetic   bool                         `json:"synthetic"`
	Accounts    []financial.FinancialAccount `json:"accounts"`
	RetrievedAt time.Time                    `json:"retrieved_at"`
}

type accountEnvelope struct {
	Response *struct {
		Accounts *struct {
			Account *[]struct {
				ID     string `json:"accountId"`
				Key    string `json:"accountIdKey"`
				Type   string `json:"accountType"`
				Status string `json:"accountStatus"`
			} `json:"Account"`
		} `json:"Accounts"`
	} `json:"AccountListResponse"`
}

func (c *Client) ListAccounts(ctx context.Context, a *Authorization) (AccountInventory, error) {
	if err := c.accessValid(a); err != nil {
		return AccountInventory{}, err
	}
	base := "https://api.etrade.com"
	if c.environment == Sandbox {
		base = "https://apisb.etrade.com"
	}
	body, status, err := c.get(ctx, base+"/v1/accounts/list", a.token, a.secret, nil, "application/json")
	if err != nil {
		return AccountInventory{}, err
	}
	if err = c.accessValid(a); err != nil {
		return AccountInventory{}, err
	}
	result := AccountInventory{Provider: "etrade", Environment: c.environment, Synthetic: c.environment == Sandbox,
		Accounts: []financial.FinancialAccount{}, RetrievedAt: c.now().UTC()}
	if status == http.StatusNoContent {
		if len(body) != 0 {
			return AccountInventory{}, failure(financial.InvalidProviderResponse)
		}
		return result, nil
	}
	var envelope accountEnvelope
	if !unambiguousJSON(body) {
		return AccountInventory{}, failure(financial.InvalidProviderResponse)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err = decoder.Decode(&envelope); err != nil || decoder.Decode(new(any)) != io.EOF ||
		envelope.Response == nil || envelope.Response.Accounts == nil || envelope.Response.Accounts.Account == nil {
		return AccountInventory{}, failure(financial.InvalidProviderResponse)
	}
	accounts := *envelope.Response.Accounts.Account
	if len(accounts) > 1000 {
		return AccountInventory{}, failure(financial.InvalidProviderResponse)
	}
	seenKeys, seenIDs := map[string]bool{}, map[string]bool{}
	for _, raw := range accounts {
		if !validSecret(raw.ID) || !validSecret(raw.Key) || !validAccountType(raw.Type) ||
			seenKeys[raw.Key] || seenIDs[raw.ID] || raw.Status != "ACTIVE" && raw.Status != "CLOSED" {
			return AccountInventory{}, failure(financial.InvalidProviderResponse)
		}
		seenKeys[raw.Key], seenIDs[raw.ID] = true, true
		masked := "••••"
		if len(raw.ID) > 4 {
			masked += raw.ID[len(raw.ID)-4:]
		}
		// Ignore free-form names/descriptions that may contain a full account ID.
		// Currency, balances, holdings, freshness and trading permissions are not
		// supplied by account discovery and must not be inferred here.
		result.Accounts = append(result.Accounts, financial.FinancialAccount{Provider: "etrade", ProviderAccountID: raw.Key,
			DisplayName: "E*TRADE Account " + masked, MaskedIdentifier: masked, AccountType: raw.Type,
			Status: strings.ToLower(raw.Status), DiscoveredAt: result.RetrievedAt,
			Capabilities: financial.Capabilities{"orders": financial.Unsupported, "options": financial.Unknown, "margin": financial.Unknown}})
	}
	return result, nil
}

func validAccountType(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, c := range value {
		if c != '_' && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}

// encoding/json normally accepts duplicate (including case-folded) members.
// Reject ambiguity before decoding any account inventory; never treat a
// conflicting response as an authoritative empty account list.
func unambiguousJSON(body []byte) bool {
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	var visit func(int) bool
	visit = func(depth int) bool {
		if depth > 64 {
			return false
		}
		token, err := d.Token()
		if err != nil {
			return false
		}
		switch token {
		case json.Delim('{'):
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				name, ok := key.(string)
				name = strings.ToLower(name)
				if err != nil || !ok || seen[name] || !visit(depth+1) {
					return false
				}
				seen[name] = true
			}
			end, err := d.Token()
			return err == nil && end == json.Delim('}')
		case json.Delim('['):
			for d.More() {
				if !visit(depth + 1) {
					return false
				}
			}
			end, err := d.Token()
			return err == nil && end == json.Delim(']')
		default:
			_, delim := token.(json.Delim)
			return !delim
		}
	}
	if !visit(0) {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}
