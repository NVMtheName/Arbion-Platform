package aiconnection

type Provider struct {
	ID              string   `json:"id"`
	Label           string   `json:"label"`
	CredentialTypes []string `json:"credential_types"`
	Capabilities    []string `json:"capabilities"`
}

type Registry struct{ providers map[string]Provider }

func DefaultRegistry() Registry {
	common := []string{"credential_verification", "model_discovery", "text_generation", "streaming"}
	items := []Provider{
		{ID: "openai", Label: "OpenAI", CredentialTypes: []string{"authorization_key"}, Capabilities: append(common, "structured_output", "tool_calling", "vision", "reasoning", "web_search")},
		{ID: "anthropic", Label: "Anthropic / Claude", CredentialTypes: []string{"api_key"}, Capabilities: append(common, "structured_output", "tool_calling", "vision", "reasoning")},
		{ID: "gemini", Label: "Google Gemini", CredentialTypes: []string{"api_key"}, Capabilities: append(common, "structured_output", "tool_calling", "vision", "reasoning")},
	}
	r := Registry{providers: make(map[string]Provider, len(items))}
	for _, item := range items {
		r.providers[item.ID] = item
	}
	return r
}

func (r Registry) Get(id string) (Provider, bool) { p, ok := r.providers[id]; return p, ok }
func (r Registry) List() []Provider {
	result := make([]Provider, 0, len(r.providers))
	for _, id := range []string{"openai", "anthropic", "gemini"} {
		if p, ok := r.providers[id]; ok {
			result = append(result, p)
		}
	}
	return result
}
