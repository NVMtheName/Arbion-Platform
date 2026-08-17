from __future__ import annotations

import json
import time
from dataclasses import dataclass

import httpx

from .models import (
    Capability,
    CredentialType,
    ErrorCode,
    Insight,
    NeuralProviderError,
    ProviderModel,
    ResponseMetadata,
    VerificationResult,
)
from .provider import NeuralProvider

MAX_RESPONSE_BYTES = 2 * 1024 * 1024
TIMEOUT = httpx.Timeout(35.0, connect=5.0)
INSIGHT_INSTRUCTIONS = """You are Arbion Insight, a read-only educational financial
analysis assistant. Answer only from the user's supplied text. You have no access to their
account, portfolio, broker, holdings, positions, market quotes, news, or real-time data.
Never claim that you placed, authorized, previewed, or will place a trade. Never provide
private chain-of-thought. If a useful answer requires current market or account data, set
requires_current_data to true and state that limitation. Be concise, identify material risk,
and do not imply guaranteed returns. Arbion's deterministic systems—not the model—authorize
any future financial action."""
INSIGHT_SCHEMA: dict[str, object] = {
    "type": "object",
    "properties": {
        "summary": {"type": "string"},
        "key_points": {"type": "array", "items": {"type": "string"}},
        "risk_flags": {"type": "array", "items": {"type": "string"}},
        "limitations": {"type": "array", "items": {"type": "string"}},
        "requires_current_data": {"type": "boolean"},
    },
    "required": [
        "summary",
        "key_points",
        "risk_flags",
        "limitations",
        "requires_current_data",
    ],
    "additionalProperties": False,
}


@dataclass(frozen=True)
class InsightRoute:
    model: str
    reasoning_effort: str
    max_output_tokens: int
    verbosity: str


INSIGHT_ROUTES: dict[str, InsightRoute] = {
    "fast": InsightRoute("gpt-5.6-luna", "low", 700, "low"),
    "core": InsightRoute("gpt-5.6-terra", "medium", 900, "low"),
    "deep": InsightRoute("gpt-5.6-sol", "high", 1200, "medium"),
}


class HTTPProvider(NeuralProvider):
    def __init__(self, client: httpx.AsyncClient | None = None) -> None:
        self._client = client

    async def _get(
        self, url: str, headers: dict[str, str], params: dict[str, str] | None = None
    ) -> dict[str, object]:
        return await self._request("GET", url, headers, params=params)

    async def _post(
        self, url: str, headers: dict[str, str], payload: dict[str, object]
    ) -> dict[str, object]:
        return await self._request("POST", url, headers, payload=payload)

    async def _request(
        self,
        method: str,
        url: str,
        headers: dict[str, str],
        params: dict[str, str] | None = None,
        payload: dict[str, object] | None = None,
    ) -> dict[str, object]:
        owned = self._client is None
        client = self._client or httpx.AsyncClient(timeout=TIMEOUT, follow_redirects=False)
        body = bytearray()
        try:
            async with client.stream(
                method, url, headers=headers, params=params, json=payload
            ) as response:
                if response.status_code in (401, 403):
                    raise NeuralProviderError(ErrorCode.AUTHENTICATION_FAILED)
                if response.status_code == 429:
                    raise NeuralProviderError(ErrorCode.RATE_LIMITED)
                if response.status_code >= 500:
                    raise NeuralProviderError(ErrorCode.PROVIDER_UNAVAILABLE)
                if response.status_code >= 400:
                    raise NeuralProviderError(ErrorCode.INVALID_REQUEST)
                async for chunk in response.aiter_bytes():
                    body.extend(chunk)
                    if len(body) > MAX_RESPONSE_BYTES:
                        raise NeuralProviderError(ErrorCode.PROVIDER_UNAVAILABLE)
            value = json.loads(body)
            if not isinstance(value, dict):
                raise NeuralProviderError(ErrorCode.PROVIDER_UNAVAILABLE)
            return value
        except httpx.TimeoutException as exc:
            raise NeuralProviderError(ErrorCode.TIMEOUT) from exc
        except (httpx.NetworkError, ValueError) as exc:
            raise NeuralProviderError(ErrorCode.PROVIDER_UNAVAILABLE) from exc
        finally:
            body.clear()
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

    async def analyze(
        self,
        credential: str,
        profile: str,
        prompt: str,
        safety_identifier: str,
    ) -> Insight:
        route = INSIGHT_ROUTES.get(profile)
        if route is None:
            raise NeuralProviderError(ErrorCode.INVALID_REQUEST)
        started = time.monotonic()
        payload = await self._post(
            "https://api.openai.com/v1/responses",
            {"Authorization": f"Bearer {credential}"},
            {
                "model": route.model,
                "instructions": INSIGHT_INSTRUCTIONS,
                "input": prompt,
                "store": False,
                "safety_identifier": safety_identifier,
                "reasoning": {"effort": route.reasoning_effort},
                "max_output_tokens": route.max_output_tokens,
                "text": {
                    "verbosity": route.verbosity,
                    "format": {
                        "type": "json_schema",
                        "name": "arbion_insight",
                        "strict": True,
                        "schema": INSIGHT_SCHEMA,
                    },
                },
            },
        )
        return self._normalize_insight(
            payload,
            profile,
            route.model,
            int((time.monotonic() - started) * 1000),
        )

    def _normalize_insight(
        self, payload: dict[str, object], profile: str, model: str, latency_ms: int
    ) -> Insight:
        if payload.get("status") != "completed":
            raise NeuralProviderError(ErrorCode.INTERNAL_ERROR)
        text = self._output_text(payload.get("output"))
        try:
            value = json.loads(text)
        except (TypeError, ValueError) as exc:
            raise NeuralProviderError(ErrorCode.INTERNAL_ERROR) from exc
        if not isinstance(value, dict):
            raise NeuralProviderError(ErrorCode.INTERNAL_ERROR)
        summary = value.get("summary")
        requires_current_data = value.get("requires_current_data")
        if not isinstance(summary, str) or not isinstance(requires_current_data, bool):
            raise NeuralProviderError(ErrorCode.INTERNAL_ERROR)
        key_points = self._bounded_strings(value.get("key_points"))
        risk_flags = self._bounded_strings(value.get("risk_flags"))
        limitations = self._bounded_strings(value.get("limitations"))
        usage = payload.get("usage")
        input_tokens = usage.get("input_tokens") if isinstance(usage, dict) else None
        output_tokens = usage.get("output_tokens") if isinstance(usage, dict) else None
        response_id = payload.get("id")
        return Insight(
            summary=summary[:2000],
            key_points=key_points,
            risk_flags=risk_flags,
            limitations=limitations,
            requires_current_data=requires_current_data,
            metadata=ResponseMetadata(
                provider=self.id,
                model=model,
                profile=profile,
                input_usage=input_tokens if isinstance(input_tokens, int) else None,
                output_usage=output_tokens if isinstance(output_tokens, int) else None,
                request_id=response_id if isinstance(response_id, str) else None,
                latency_ms=latency_ms,
            ),
        )

    @staticmethod
    def _output_text(output: object) -> str:
        if not isinstance(output, list):
            raise NeuralProviderError(ErrorCode.INTERNAL_ERROR)
        for item in output:
            if not isinstance(item, dict) or item.get("type") != "message":
                continue
            content = item.get("content")
            if not isinstance(content, list):
                continue
            for part in content:
                if isinstance(part, dict) and part.get("type") == "output_text":
                    text = part.get("text")
                    if isinstance(text, str):
                        return text
        raise NeuralProviderError(ErrorCode.INTERNAL_ERROR)

    @staticmethod
    def _bounded_strings(value: object) -> tuple[str, ...]:
        if not isinstance(value, list) or not all(isinstance(item, str) for item in value):
            raise NeuralProviderError(ErrorCode.INTERNAL_ERROR)
        return tuple(item[:500] for item in value[:8])


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
