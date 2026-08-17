import json

import httpx
import pytest

from app.neural.adapters import (
    MAX_RESPONSE_BYTES,
    AnthropicProvider,
    GeminiProvider,
    OpenAIProvider,
)
from app.neural.models import ErrorCode, NeuralProviderError
from app.neural.registry import default_registry


def client(handler: httpx.AsyncBaseTransport) -> httpx.AsyncClient:
    return httpx.AsyncClient(transport=handler)


@pytest.mark.parametrize(
    ("provider_type", "payload", "expected"),
    [
        (OpenAIProvider, {"data": [{"id": "model-a"}]}, "model-a"),
        (
            AnthropicProvider,
            {"data": [{"id": "claude-test", "display_name": "Claude Test"}]},
            "claude-test",
        ),
        (
            GeminiProvider,
            {"models": [{"name": "models/gemini-test", "displayName": "Gemini Test"}]},
            "gemini-test",
        ),
    ],
)
@pytest.mark.asyncio
async def test_adapters_verify_and_normalize_models(provider_type, payload, expected):
    async def respond(request: httpx.Request) -> httpx.Response:
        assert any(value.endswith("secret-value") for value in request.headers.values())
        return httpx.Response(200, json=payload)

    async with client(httpx.MockTransport(respond)) as http:
        provider = provider_type(http)
        assert (await provider.verify_credentials("secret-value")).valid
        models = await provider.list_models("secret-value")
        assert models[0].id == expected
        assert models[0].provider == provider.id


@pytest.mark.parametrize(
    ("status", "code"),
    [
        (401, ErrorCode.AUTHENTICATION_FAILED),
        (429, ErrorCode.RATE_LIMITED),
        (503, ErrorCode.PROVIDER_UNAVAILABLE),
    ],
)
@pytest.mark.asyncio
async def test_errors_are_normalized_without_provider_body(status, code):
    secret = "never-return-this"
    transport = httpx.MockTransport(lambda request: httpx.Response(status, text=secret))
    async with client(transport) as http:
        with pytest.raises(NeuralProviderError) as caught:
            await OpenAIProvider(http).verify_credentials(secret)
    assert caught.value.code == code
    assert secret not in str(caught.value)


def test_registry_contains_each_adapter():
    registry = default_registry()
    assert registry.get("openai").id == "openai"
    assert registry.get("anthropic").id == "anthropic"
    assert registry.get("gemini").id == "gemini"


@pytest.mark.asyncio
async def test_openai_insight_uses_bounded_structured_response() -> None:
    async def respond(request: httpx.Request) -> httpx.Response:
        assert request.url == "https://api.openai.com/v1/responses"
        body = json.loads(request.content)
        assert body["model"] == "gpt-5.6-luna"
        assert body["store"] is False
        assert body["reasoning"] == {"effort": "low"}
        assert body["text"]["format"]["type"] == "json_schema"
        assert body["text"]["format"]["strict"] is True
        assert "tools" not in body
        assert body["safety_identifier"] == "a" * 64
        return httpx.Response(
            200,
            json={
                "id": "resp-safe",
                "status": "completed",
                "output": [
                    {
                        "type": "message",
                        "content": [
                            {
                                "type": "output_text",
                                "text": json.dumps(
                                    {
                                        "summary": "Diversification can reduce concentration risk.",
                                        "key_points": ["Review allocation across asset classes."],
                                        "risk_flags": ["Concentration risk"],
                                        "limitations": [
                                            "No account or live market data was supplied."
                                        ],
                                        "requires_current_data": False,
                                    }
                                ),
                            }
                        ],
                    }
                ],
                "usage": {"input_tokens": 30, "output_tokens": 45},
            },
        )

    async with client(httpx.MockTransport(respond)) as http:
        result = await OpenAIProvider(http).analyze(
            "secret-value", "gpt-5.6-luna", "How should I think about diversification?", "a" * 64
        )
    assert result.summary.startswith("Diversification")
    assert result.metadata.input_usage == 30
    assert result.metadata.output_usage == 45
    assert result.metadata.request_id == "resp-safe"


@pytest.mark.asyncio
async def test_openai_insight_rejects_unstructured_provider_output() -> None:
    transport = httpx.MockTransport(
        lambda request: httpx.Response(
            200,
            json={
                "id": "resp-invalid",
                "status": "completed",
                "output": [
                    {"type": "message", "content": [{"type": "output_text", "text": "not-json"}]}
                ],
            },
        )
    )
    async with client(transport) as http:
        with pytest.raises(NeuralProviderError) as caught:
            await OpenAIProvider(http).analyze("secret-value", "gpt-5.6-luna", "question", "a" * 64)
    assert caught.value.code == ErrorCode.INTERNAL_ERROR


@pytest.mark.asyncio
async def test_provider_response_is_rejected_at_streaming_size_limit() -> None:
    transport = httpx.MockTransport(
        lambda request: httpx.Response(200, content=b"x" * (MAX_RESPONSE_BYTES + 1))
    )
    async with client(transport) as http:
        with pytest.raises(NeuralProviderError) as caught:
            await OpenAIProvider(http).verify_credentials("secret-value")
    assert caught.value.code == ErrorCode.PROVIDER_UNAVAILABLE
