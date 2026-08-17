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
