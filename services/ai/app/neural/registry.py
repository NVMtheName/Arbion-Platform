from __future__ import annotations

import httpx

from .adapters import AnthropicProvider, GeminiProvider, OpenAIProvider
from .provider import NeuralProvider


class ProviderRegistry:
    def __init__(self, providers: list[NeuralProvider]) -> None:
        self._providers = {provider.id: provider for provider in providers}

    def get(self, provider_id: str) -> NeuralProvider:
        try:
            return self._providers[provider_id]
        except KeyError as exc:
            raise ValueError("unsupported provider") from exc


def default_registry(client: httpx.AsyncClient | None = None) -> ProviderRegistry:
    return ProviderRegistry(
        [OpenAIProvider(client), AnthropicProvider(client), GeminiProvider(client)]
    )
