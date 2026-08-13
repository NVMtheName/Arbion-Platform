package aiconnection

type Provider struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type Registry struct{ providers map[string]Provider }

func DefaultRegistry() Registry {
	items := []Provider{{"openai", "OpenAI"}, {"anthropic", "Anthropic / Claude"}, {"gemini", "Google Gemini"}}
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
