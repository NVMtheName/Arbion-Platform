import httpx
import pytest

from app.neural.adapters import AnthropicProvider, GeminiProvider, OpenAIProvider
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
