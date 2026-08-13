from __future__ import annotations

import httpx

from .models import (
    Capability,
    CredentialType,
    ErrorCode,
    NeuralProviderError,
    ProviderModel,
    VerificationResult,
)
from .provider import NeuralProvider

MAX_RESPONSE_BYTES = 2 * 1024 * 1024
TIMEOUT = httpx.Timeout(10.0, connect=5.0)


class HTTPProvider(NeuralProvider):
    def __init__(self, client: httpx.AsyncClient | None = None) -> None:
        self._client = client

    async def _get(
        self, url: str, headers: dict[str, str], params: dict[str, str] | None = None
    ) -> dict[str, object]:
        owned = self._client is None
        client = self._client or httpx.AsyncClient(timeout=TIMEOUT, follow_redirects=False)
        try:
            response = await client.get(url, headers=headers, params=params)
            if len(response.content) > MAX_RESPONSE_BYTES:
                raise NeuralProviderError(ErrorCode.PROVIDER_UNAVAILABLE)
            if response.status_code in (401, 403):
                raise NeuralProviderError(ErrorCode.AUTHENTICATION_FAILED)
            if response.status_code == 429:
                raise NeuralProviderError(ErrorCode.RATE_LIMITED)
            if response.status_code >= 500:
                raise NeuralProviderError(ErrorCode.PROVIDER_UNAVAILABLE)
            if response.status_code >= 400:
                raise NeuralProviderError(ErrorCode.INVALID_REQUEST)
            value = response.json()
            if not isinstance(value, dict):
                raise NeuralProviderError(ErrorCode.PROVIDER_UNAVAILABLE)
            return value
        except httpx.TimeoutException as exc:
            raise NeuralProviderError(ErrorCode.TIMEOUT) from exc
        except (httpx.NetworkError, ValueError) as exc:
            raise NeuralProviderError(ErrorCode.PROVIDER_UNAVAILABLE) from exc
        finally:
            if owned:
                await client.aclose()

    async def verify_credentials(self, credential: str) -> VerificationResult:
        await self._models_payload(credential, verify_only=True)
        return VerificationResult(provider=self.id)

    async def list_models(self, credential: str) -> list[ProviderModel]:
        payload = await self._models_payload(credential, verify_only=False)
        return self._normalize(payload)

    async def _models_payload(self, credential: str, verify_only: bool) -> dict[str, object]:
        raise NotImplementedError

    def _normalize(self, payload: dict[str, object]) -> list[ProviderModel]:
        raw = payload.get("data", payload.get("models", []))
        if not isinstance(raw, list):
            raise NeuralProviderError(ErrorCode.PROVIDER_UNAVAILABLE)
        result: list[ProviderModel] = []
        for item in raw:
            if not isinstance(item, dict):
                continue
            model_id = item.get("id", item.get("name"))
            if not isinstance(model_id, str) or not model_id:
                continue
            if model_id.startswith("models/"):
                model_id = model_id.removeprefix("models/")
            display = item.get("displayName", item.get("display_name", model_id))
            result.append(ProviderModel(model_id, str(display), self.id))
        return result


COMMON = frozenset(
    {
        Capability.CREDENTIAL_VERIFICATION,
        Capability.MODEL_DISCOVERY,
        Capability.TEXT_GENERATION,
        Capability.STREAMING,
    }
)


class OpenAIProvider(HTTPProvider):
    id = "openai"
    credential_types = (CredentialType.AUTHORIZATION_KEY,)
    capabilities = COMMON | {
        Capability.STRUCTURED_OUTPUT,
        Capability.TOOL_CALLING,
        Capability.VISION,
        Capability.REASONING,
        Capability.WEB_SEARCH,
    }

    async def _models_payload(self, credential: str, verify_only: bool) -> dict[str, object]:
        return await self._get(
            "https://api.openai.com/v1/models", {"Authorization": f"Bearer {credential}"}
        )


class AnthropicProvider(HTTPProvider):
    id = "anthropic"
    credential_types = (CredentialType.API_KEY,)
    capabilities = COMMON | {
        Capability.STRUCTURED_OUTPUT,
        Capability.TOOL_CALLING,
        Capability.VISION,
        Capability.REASONING,
    }

    async def _models_payload(self, credential: str, verify_only: bool) -> dict[str, object]:
        params = {"limit": "1"} if verify_only else None
        return await self._get(
            "https://api.anthropic.com/v1/models",
            {"x-api-key": credential, "anthropic-version": "2023-06-01"},
            params,
        )


class GeminiProvider(HTTPProvider):
    id = "gemini"
    credential_types = (CredentialType.API_KEY,)
    capabilities = COMMON | {
        Capability.STRUCTURED_OUTPUT,
        Capability.TOOL_CALLING,
        Capability.VISION,
        Capability.REASONING,
    }

    async def _models_payload(self, credential: str, verify_only: bool) -> dict[str, object]:
        params = {"pageSize": "1"} if verify_only else None
        return await self._get(
            "https://generativelanguage.googleapis.com/v1beta/models",
            {"x-goog-api-key": credential},
            params,
        )
