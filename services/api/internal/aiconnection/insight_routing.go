package aiconnection

import "strings"

type InsightProfile string

const (
	InsightProfileFast InsightProfile = "fast"
	InsightProfileCore InsightProfile = "core"
	InsightProfileDeep InsightProfile = "deep"
)

type InsightRoute struct {
	Profile     InsightProfile
	Provider    string
	ModelID     string
	CreditUnits int
}

var insightRoutes = map[InsightProfile]InsightRoute{
	InsightProfileFast: {
		Profile: InsightProfileFast, Provider: "openai", ModelID: "gpt-5.6-luna", CreditUnits: 1,
	},
	InsightProfileCore: {
		Profile: InsightProfileCore, Provider: "openai", ModelID: "gpt-5.6-terra", CreditUnits: 3,
	},
	InsightProfileDeep: {
		Profile: InsightProfileDeep, Provider: "openai", ModelID: "gpt-5.6-sol", CreditUnits: 6,
	},
}

var shadowModelRoutes = map[string]InsightRoute{
	"gpt-5.6-luna": {
		Profile: InsightProfileFast, Provider: "openai", ModelID: "gpt-5.6-luna", CreditUnits: 1,
	},
	"gpt-5.6-terra": {
		Profile: InsightProfileCore, Provider: "openai", ModelID: "gpt-5.6-terra", CreditUnits: 3,
	},
	"gpt-5.6-sol": {
		Profile: InsightProfileDeep, Provider: "openai", ModelID: "gpt-5.6-sol", CreditUnits: 6,
	},
	"claude-haiku-4-5-20251001": {
		Profile: InsightProfileFast, Provider: "anthropic", ModelID: "claude-haiku-4-5-20251001", CreditUnits: 1,
	},
	"claude-sonnet-5": {
		Profile: InsightProfileCore, Provider: "anthropic", ModelID: "claude-sonnet-5", CreditUnits: 3,
	},
	"claude-opus-5": {
		Profile: InsightProfileDeep, Provider: "anthropic", ModelID: "claude-opus-5", CreditUnits: 6,
	},
	"gemini-3.5-flash": {
		Profile: InsightProfileFast, Provider: "gemini", ModelID: "gemini-3.5-flash", CreditUnits: 1,
	},
	"gemini-3.6-flash": {
		Profile: InsightProfileCore, Provider: "gemini", ModelID: "gemini-3.6-flash", CreditUnits: 3,
	},
	"gemini-3.7-flash": {
		Profile: InsightProfileDeep, Provider: "gemini", ModelID: "gemini-3.7-flash", CreditUnits: 6,
	},
}

func resolveInsightRoute(raw string) (InsightRoute, error) {
	profile := InsightProfile(strings.ToLower(strings.TrimSpace(raw)))
	if profile == "" {
		profile = InsightProfileFast
	}
	route, ok := insightRoutes[profile]
	if !ok {
		return InsightRoute{}, ErrInvalid
	}
	return route, nil
}

func resolveModelRoute(modelID string) (InsightRoute, error) {
	modelID = strings.TrimSpace(modelID)
	if route, ok := shadowModelRoutes[modelID]; ok {
		return route, nil
	}
	return InsightRoute{}, ErrInvalid
}
