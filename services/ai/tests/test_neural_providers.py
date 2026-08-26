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


@pytest.mark.parametrize(
    ("profile", "model", "effort", "max_output_tokens", "verbosity"),
    [
        ("fast", "gpt-5.6-luna", "low", 700, "low"),
        ("core", "gpt-5.6-terra", "medium", 900, "low"),
        ("deep", "gpt-5.6-sol", "high", 1200, "medium"),
    ],
)
@pytest.mark.asyncio
async def test_openai_insight_uses_bounded_structured_response(
    profile: str,
    model: str,
    effort: str,
    max_output_tokens: int,
    verbosity: str,
) -> None:
    async def respond(request: httpx.Request) -> httpx.Response:
        assert request.url == "https://api.openai.com/v1/responses"
        body = json.loads(request.content)
        assert body["model"] == model
        assert body["store"] is False
        assert body["reasoning"] == {"effort": effort}
        assert body["max_output_tokens"] == max_output_tokens
        assert body["text"]["verbosity"] == verbosity
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
            "secret-value", profile, "How should I think about diversification?", "a" * 64
        )
    assert result.summary.startswith("Diversification")
    assert result.metadata.input_usage == 30
    assert result.metadata.output_usage == 45
    assert result.metadata.request_id == "resp-safe"
    assert result.metadata.model == model
    assert result.metadata.profile == profile


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
            await OpenAIProvider(http).analyze("secret-value", "fast", "question", "a" * 64)
    assert caught.value.code == ErrorCode.INTERNAL_ERROR


@pytest.mark.asyncio
async def test_openai_insight_rejects_unknown_profile_without_provider_call() -> None:
    called = False

    async def respond(_: httpx.Request) -> httpx.Response:
        nonlocal called
        called = True
        return httpx.Response(500)

    async with client(httpx.MockTransport(respond)) as http:
        with pytest.raises(NeuralProviderError) as caught:
            await OpenAIProvider(http).analyze("secret-value", "unknown", "question", "a" * 64)
    assert caught.value.code == ErrorCode.INVALID_REQUEST
    assert called is False


@pytest.mark.asyncio
async def test_openai_trade_proposal_is_structured_bounded_and_tool_free() -> None:
    context = {
        "objective": "Keep a small long-term BTC allocation.",
        "symbol": "BTC",
        "side": "BUY",
        "max_size": "50",
        "max_size_unit": "USD",
        "available_cash_usd": "200",
        "position_quantity": "0.012",
        "position_available_quantity": "0.01",
        "observed_at": "2026-08-24T12:00:00+00:00",
    }

    async def respond(request: httpx.Request) -> httpx.Response:
        body = json.loads(request.content)
        assert request.url == "https://api.openai.com/v1/responses"
        assert body["model"] == "gpt-5.6-terra"
        assert body["store"] is False
        assert body["reasoning"] == {"effort": "medium"}
        assert body["max_output_tokens"] == 900
        assert body["text"]["format"]["type"] == "json_schema"
        assert body["text"]["format"]["strict"] is True
        assert body["text"]["format"]["schema"]["additionalProperties"] is False
        assert "tools" not in body
        assert json.loads(body["input"]) == context
        return httpx.Response(
            200,
            json={
                "id": "resp-proposal-safe",
                "status": "completed",
                "output": [
                    {
                        "type": "message",
                        "content": [
                            {
                                "type": "output_text",
                                "text": json.dumps(
                                    {
                                        "decision": "PROPOSE",
                                        "requested_size": "25.50",
                                        "confidence": "LOW",
                                        "thesis": (
                                            "A smaller amount stays within the user's "
                                            "fixed ceiling."
                                        ),
                                        "risk_flags": ["Crypto prices can move sharply."],
                                        "limitations": [
                                            "No news or external market feed was supplied."
                                        ],
                                    }
                                ),
                            }
                        ],
                    }
                ],
                "usage": {"input_tokens": 80, "output_tokens": 55},
            },
        )

    async with client(httpx.MockTransport(respond)) as http:
        result = await OpenAIProvider(http).propose_trade("secret-value", "core", context, "a" * 64)
    assert result.decision == "PROPOSE"
    assert result.requested_size == "25.50"
    assert result.metadata.provider == "openai"
    assert result.metadata.model == "gpt-5.6-terra"
    assert result.metadata.profile == "core"


@pytest.mark.asyncio
async def test_openai_trade_proposal_rejects_size_above_user_ceiling() -> None:
    transport = httpx.MockTransport(
        lambda request: httpx.Response(
            200,
            json={
                "id": "resp-unsafe",
                "status": "completed",
                "output": [
                    {
                        "type": "message",
                        "content": [
                            {
                                "type": "output_text",
                                "text": json.dumps(
                                    {
                                        "decision": "PROPOSE",
                                        "requested_size": "51",
                                        "confidence": "HIGH",
                                        "thesis": "Unsafe size",
                                        "risk_flags": [],
                                        "limitations": [],
                                    }
                                ),
                            }
                        ],
                    }
                ],
            },
        )
    )
    async with client(transport) as http:
        with pytest.raises(NeuralProviderError) as caught:
            await OpenAIProvider(http).propose_trade(
                "secret-value",
                "core",
                {
                    "max_size": "50",
                },
                "a" * 64,
            )
    assert caught.value.code == ErrorCode.INTERNAL_ERROR


@pytest.mark.asyncio
async def test_openai_shadow_decision_is_structured_tool_free_and_bounded() -> None:
    context: dict[str, object] = {
        "objective": "Preserve capital.",
        "allowed_symbols": ["BTC", "ETH"],
        "max_proposal_notional": "1",
        "available_cash_usd": "100",
        "buying_power_usd": "100",
        "positions": [],
        "markets": [{"symbol": "BTC", "bid": "99", "ask": "101"}],
        "recent_decisions": [
            {
                "decision": "PROPOSE",
                "symbol": "BTC",
                "side": "BUY",
                "disposition": "WOULD_HAVE_SUBMITTED",
                "occurred_at": "2026-08-25T13:30:00+00:00",
            }
        ],
        "observed_at": "2026-08-25T14:00:00+00:00",
    }

    async def respond(request: httpx.Request) -> httpx.Response:
        body = json.loads(request.content)
        assert body["model"] == "gpt-5.6-sol"
        assert body["store"] is False
        assert body["reasoning"] == {"effort": "high"}
        assert body["text"]["format"]["strict"] is True
        assert body["text"]["format"]["schema"]["additionalProperties"] is False
        assert "tools" not in body
        assert json.loads(body["input"]) == context
        return httpx.Response(
            200,
            json={
                "id": "resp-shadow",
                "status": "completed",
                "output": [
                    {
                        "type": "message",
                        "content": [
                            {
                                "type": "output_text",
                                "text": json.dumps(
                                    {
                                        "decision": "PROPOSE",
                                        "symbol": "BTC",
                                        "side": "BUY",
                                        "proposed_notional": "1",
                                        "confidence": "LOW",
                                        "thesis": (
                                            "The bounded proposal stays within the fixed ceiling."
                                        ),
                                        "risk_flags": ["Volatility"],
                                        "limitations": ["No news feed"],
                                    }
                                ),
                            }
                        ],
                    }
                ],
                "usage": {"input_tokens": 120, "output_tokens": 60},
            },
        )

    async with client(httpx.MockTransport(respond)) as http:
        result = await OpenAIProvider(http).propose_shadow(
            "secret-value", "deep", context, "a" * 64
        )
    assert result.decision == "PROPOSE"
    assert result.proposed_notional == "1"
    assert result.metadata.model == "gpt-5.6-sol"


@pytest.mark.asyncio
async def test_openai_shadow_decision_rejects_symbol_outside_mandate() -> None:
    transport = httpx.MockTransport(
        lambda request: httpx.Response(
            200,
            json={
                "id": "resp-unsafe",
                "status": "completed",
                "output": [
                    {
                        "type": "message",
                        "content": [
                            {
                                "type": "output_text",
                                "text": json.dumps(
                                    {
                                        "decision": "PROPOSE",
                                        "symbol": "DOGE",
                                        "side": "BUY",
                                        "proposed_notional": "1",
                                        "confidence": "LOW",
                                        "thesis": "Unsafe",
                                        "risk_flags": [],
                                        "limitations": [],
                                    }
                                ),
                            }
                        ],
                    }
                ],
            },
        )
    )
    context: dict[str, object] = {"allowed_symbols": ["BTC"], "max_proposal_notional": "1"}
    async with client(transport) as http:
        with pytest.raises(NeuralProviderError) as caught:
            await OpenAIProvider(http).propose_shadow("secret-value", "deep", context, "a" * 64)
    assert caught.value.code == ErrorCode.INTERNAL_ERROR


@pytest.mark.asyncio
async def test_anthropic_shadow_decision_is_structured_tool_free_and_bounded() -> None:
    context: dict[str, object] = {
        "objective": "Preserve capital.",
        "allowed_symbols": ["BTC"],
        "max_proposal_notional": "1",
        "available_cash_usd": "100",
        "buying_power_usd": "100",
        "positions": [],
        "markets": [{"symbol": "BTC", "bid": "99", "ask": "101"}],
        "recent_decisions": [],
        "observed_at": "2026-08-25T14:00:00+00:00",
    }

    async def respond(request: httpx.Request) -> httpx.Response:
        body = json.loads(request.content)
        assert request.url == "https://api.anthropic.com/v1/messages"
        assert request.headers["x-api-key"] == "secret-value"
        assert body["model"] == "claude-sonnet-5"
        assert body["system"]
        assert body["metadata"] == {"user_id": "a" * 64}
        assert body["output_config"]["format"]["type"] == "json_schema"
        assert body["output_config"]["format"]["schema"]["additionalProperties"] is False
        assert "tools" not in body
        assert json.loads(body["messages"][0]["content"]) == context
        return httpx.Response(
            200,
            json={
                "id": "msg-shadow",
                "type": "message",
                "model": "claude-sonnet-5",
                "stop_reason": "end_turn",
                "content": [
                    {
                        "type": "text",
                        "text": json.dumps(
                            {
                                "decision": "ABSTAIN",
                                "symbol": "NONE",
                                "side": "NONE",
                                "proposed_notional": "0",
                                "confidence": "LOW",
                                "thesis": (
                                    "The bounded evidence does not support a cautious action."
                                ),
                                "risk_flags": ["Volatility"],
                                "limitations": ["No news feed"],
                            }
                        ),
                    }
                ],
                "usage": {"input_tokens": 100, "output_tokens": 40},
            },
        )

    async with client(httpx.MockTransport(respond)) as http:
        result = await AnthropicProvider(http).propose_shadow(
            "secret-value", "core", context, "a" * 64
        )
    assert result.decision == "ABSTAIN"
    assert result.metadata.provider == "anthropic"
    assert result.metadata.model == "claude-sonnet-5"
    assert result.metadata.profile == "core"
    assert result.metadata.request_id == "msg-shadow"


@pytest.mark.asyncio
async def test_anthropic_shadow_decision_rejects_truncation() -> None:
    transport = httpx.MockTransport(
        lambda request: httpx.Response(
            200,
            json={
                "id": "msg-truncated",
                "type": "message",
                "model": "claude-opus-5",
                "stop_reason": "max_tokens",
                "content": [{"type": "text", "text": "{}"}],
            },
        )
    )
    context: dict[str, object] = {"allowed_symbols": ["BTC"], "max_proposal_notional": "1"}
    async with client(transport) as http:
        with pytest.raises(NeuralProviderError) as caught:
            await AnthropicProvider(http).propose_shadow("secret-value", "deep", context, "a" * 64)
    assert caught.value.code == ErrorCode.INTERNAL_ERROR


@pytest.mark.asyncio
async def test_gemini_shadow_decision_is_structured_stored_off_and_tool_free() -> None:
    context: dict[str, object] = {
        "objective": "Preserve capital.",
        "allowed_symbols": ["BTC"],
        "max_proposal_notional": "1",
        "available_cash_usd": "100",
        "buying_power_usd": "100",
        "positions": [],
        "markets": [{"symbol": "BTC", "bid": "99", "ask": "101"}],
        "recent_decisions": [],
        "observed_at": "2026-08-25T14:00:00+00:00",
    }

    async def respond(request: httpx.Request) -> httpx.Response:
        body = json.loads(request.content)
        assert request.url == "https://generativelanguage.googleapis.com/v1beta/interactions"
        assert request.headers["x-goog-api-key"] == "secret-value"
        assert body["model"] == "gemini-3.7-flash"
        assert body["store"] is False
        assert body["background"] is False
        assert body["labels"] == {"arbion_safety_id": "a" * 64}
        assert body["generation_config"] == {
            "max_output_tokens": 900,
            "thinking_level": "high",
            "thinking_summaries": "none",
            "tool_choice": "none",
        }
        assert body["response_format"]["mime_type"] == "application/json"
        assert body["response_format"]["schema"]["additionalProperties"] is False
        assert "tools" not in body
        assert json.loads(body["input"]) == context
        return httpx.Response(
            200,
            json={
                "id": "interaction-shadow",
                "model": "gemini-3.7-flash",
                "status": "completed",
                "steps": [
                    {
                        "type": "model_output",
                        "content": [
                            {
                                "type": "text",
                                "text": json.dumps(
                                    {
                                        "decision": "PROPOSE",
                                        "symbol": "BTC",
                                        "side": "BUY",
                                        "proposed_notional": "1",
                                        "confidence": "LOW",
                                        "thesis": "A bounded proposal fits the mandate.",
                                        "risk_flags": ["Volatility"],
                                        "limitations": ["No news feed"],
                                    }
                                ),
                            }
                        ],
                    }
                ],
                "usage": {
                    "total_input_tokens": 90,
                    "total_output_tokens": 35,
                    "total_tool_use_tokens": 0,
                },
            },
        )

    async with client(httpx.MockTransport(respond)) as http:
        result = await GeminiProvider(http).propose_shadow(
            "secret-value", "deep", context, "a" * 64
        )
    assert result.decision == "PROPOSE"
    assert result.metadata.provider == "gemini"
    assert result.metadata.model == "gemini-3.7-flash"
    assert result.metadata.input_usage == 90
    assert result.metadata.request_id == "interaction-shadow"


@pytest.mark.asyncio
async def test_gemini_shadow_decision_rejects_incomplete_interaction() -> None:
    transport = httpx.MockTransport(
        lambda request: httpx.Response(
            200,
            json={
                "id": "interaction-incomplete",
                "model": "gemini-3.6-flash",
                "status": "incomplete",
                "steps": [],
            },
        )
    )
    context: dict[str, object] = {"allowed_symbols": ["BTC"], "max_proposal_notional": "1"}
    async with client(transport) as http:
        with pytest.raises(NeuralProviderError) as caught:
            await GeminiProvider(http).propose_shadow("secret-value", "core", context, "a" * 64)
    assert caught.value.code == ErrorCode.INTERNAL_ERROR


@pytest.mark.asyncio
async def test_gemini_shadow_decision_rejects_tool_use() -> None:
    transport = httpx.MockTransport(
        lambda request: httpx.Response(
            200,
            json={
                "id": "interaction-tool-use",
                "model": "gemini-3.6-flash",
                "status": "completed",
                "steps": [{"type": "function_call", "name": "place_order"}],
                "usage": {"total_tool_use_tokens": 1},
            },
        )
    )
    context: dict[str, object] = {"allowed_symbols": ["BTC"], "max_proposal_notional": "1"}
    async with client(transport) as http:
        with pytest.raises(NeuralProviderError) as caught:
            await GeminiProvider(http).propose_shadow("secret-value", "core", context, "a" * 64)
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
