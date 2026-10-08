import copy
import json
from typing import Any

import httpx
import pytest
from fastapi.testclient import TestClient
from pydantic import ValidationError

from app import main
from app.main import LivePilotDecisionRequest, ShadowDecisionRequest
from app.neural.adapters import OpenAIProvider
from app.neural.models import ErrorCode, NeuralProviderError, ResponseMetadata, ShadowDecision


def pilot_request() -> dict[str, Any]:
    return {
        "provider": "openai",
        "credential": "synthetic-secret",
        "profile": "deep",
        "objective": "Only the isolated pilot after costs.",
        "allowed_symbols": ["BTC"],
        "max_proposal_notional": "24",
        "available_cash_usd": "100",
        "buying_power_usd": "100",
        "positions": [],
        "markets": [
            {
                "symbol": "BTC",
                "asset_class": "CRYPTO",
                "currency": "USD",
                "bid": "60000",
                "ask": "60001",
                "mark": "",
                "last": "",
                "change_percent_1h": "",
                "change_percent_6h": "",
                "change_percent_24h": "",
                "volume_24h": "",
                "feed": "Coinbase",
                "quality": "LIVE",
                "observed_at": "2026-10-07T12:00:00Z",
                "history_status": "UNAVAILABLE",
                "history_granularity_seconds": 0,
                "history_contiguous_intervals": 0,
                "history_expected_intervals": 0,
                "history_feed": "",
                "history_quality": "",
            }
        ],
        "observed_at": "2026-10-07T12:00:00Z",
        "safety_identifier": "a" * 64,
    }


def decision() -> dict[str, Any]:
    return {
        "decision": "ABSTAIN",
        "symbol": "NONE",
        "side": "NONE",
        "proposed_notional": "0",
        "confidence": "LOW",
        "thesis": "No cautious edge after costs.",
        "risk_flags": [],
        "limitations": ["No history"],
    }


def provider_response(text: str | None = None) -> dict[str, Any]:
    return {
        "id": "synthetic-response",
        "status": "completed",
        "model": "gpt-5.6-sol",
        "output": [
            {
                "type": "message",
                "content": [
                    {
                        "type": "output_text",
                        "text": json.dumps(decision()) if text is None else text,
                    }
                ],
            }
        ],
        "usage": {"input_tokens": 100, "output_tokens": 50},
    }


def test_live_pilot_input_is_exact_single_pair_and_long_objective_is_not_truncated() -> None:
    payload = pilot_request()
    payload["objective"] = "x" * 2000
    request = LivePilotDecisionRequest.model_validate(payload)
    assert request.objective == payload["objective"]
    with pytest.raises(ValidationError):
        ShadowDecisionRequest.model_validate(payload)
    for change in [
        {"provider": "anthropic"},
        {"provider": "gemini"},
        {"allowed_symbols": ["BTC", "ETH"]},
        {"allowed_symbols": ["USD"]},
        {"allowed_symbols": ["btc"]},
        {"objective": " "},
        {"objective": "x" * 2001},
        {"buying_power_usd": "1000"},
        {"account_id": "private"},
        {"budget_scope": "private"},
        {"purpose": "SHADOW"},
        {"observed_at": "2026-10-07T12:00:00"},
        {"recent_decisions": [{"decision": "ABSTAIN"}]},
        {"market_events": [{"symbol": "BTC"}]},
    ]:
        with pytest.raises(ValidationError):
            LivePilotDecisionRequest.model_validate(pilot_request() | change)


@pytest.mark.parametrize(
    "change",
    [
        {"account_id": "private"},
        {"asset_class": "EQUITY"},
        {"symbol": "ETH"},
        {"currency": "EUR"},
        {"bid": "60002"},
        {"ask": ""},
        {"bid": "0"},
        {"mark": "60000"},
        {"change_percent_1h": "1"},
        {"bid_depth_usd": "10000"},
        {"observed_at": "2026-10-07T12:00:01Z"},
        {"history_status": "COMPLETE"},
    ],
)
def test_live_pilot_rejects_foreign_or_invented_market_facts(change: dict[str, Any]) -> None:
    payload = pilot_request()
    payload["markets"][0].update(change)
    with pytest.raises(ValidationError):
        LivePilotDecisionRequest.model_validate(payload)


def test_live_pilot_position_is_only_available_pilot_holding() -> None:
    payload = pilot_request()
    holding = {
        "symbol": "BTC",
        "instrument": "CRYPTO",
        "quantity": "0.0001",
        "available_quantity": "0.0001",
        "market_value_usd": "6",
        "performance_status": "UNAVAILABLE",
    }
    payload["positions"] = [holding]
    assert LivePilotDecisionRequest.model_validate(payload).positions[0].quantity == "0.0001"
    for change in [
        {"quantity": "-1"},
        {"available_quantity": "1"},
        {"symbol": "ETH"},
        {"instrument": "EQUITY"},
        {"account_id": "private"},
    ]:
        payload["positions"] = [holding | change]
        with pytest.raises(ValidationError):
            LivePilotDecisionRequest.model_validate(payload)


@pytest.mark.asyncio
async def test_live_pilot_fresh_openai_request_has_distinct_fixed_purpose_and_no_tools() -> None:
    context = LivePilotDecisionRequest.model_validate(pilot_request()).model_dump(
        exclude={"provider", "credential", "profile", "safety_identifier"}, mode="json"
    )
    calls = []

    def respond(request: httpx.Request) -> httpx.Response:
        body = json.loads(request.content)
        calls.append(body)
        assert request.url == "https://api.openai.com/v1/responses"
        assert request.headers["authorization"] == "Bearer synthetic-secret"
        assert body["model"] == "gpt-5.6-sol"
        assert body["store"] is False
        assert "tools" not in body
        assert "LIVE-purpose" in body["instructions"]
        assert "no previous non-live output may be promoted" in body["instructions"]
        assert "no tools, broker access or authority" in body["instructions"]
        assert body["text"]["format"]["name"] == "arbion_live_pilot_decision"
        assert body["text"]["format"]["strict"] is True
        assert body["text"]["format"]["schema"]["additionalProperties"] is False
        assert json.loads(body["input"]) == context
        assert "synthetic-secret" not in body["input"]
        assert "account_id" not in body["input"]
        return httpx.Response(200, json=provider_response())

    async with httpx.AsyncClient(transport=httpx.MockTransport(respond)) as http:
        result = await OpenAIProvider(http).propose_live_pilot(
            "synthetic-secret", "deep", context, "a" * 64
        )
    assert len(calls) == 1
    assert result.decision == "ABSTAIN"
    assert result.metadata.model == "gpt-5.6-sol"


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "text",
    [
        json.dumps(decision() | {"account_id": "private"}),
        json.dumps({key: value for key, value in decision().items() if key != "limitations"}),
        json.dumps(decision()).replace(
            '"decision": "ABSTAIN"', '"decision": "PROPOSE", "decision": "ABSTAIN"'
        ),
        json.dumps(decision() | {"side": "BUY"}),
        json.dumps(
            decision()
            | {"decision": "PROPOSE", "symbol": "ETH", "side": "BUY", "proposed_notional": "1"}
        ),
        json.dumps(
            decision()
            | {"decision": "PROPOSE", "symbol": "BTC", "side": "BUY", "proposed_notional": "25"}
        ),
        json.dumps(decision() | {"risk_flags": None}),
    ],
)
async def test_live_pilot_rejects_ambiguous_or_authority_bearing_output(text: str) -> None:
    async with httpx.AsyncClient(
        transport=httpx.MockTransport(
            lambda request: httpx.Response(200, json=provider_response(text))
        )
    ) as http:
        with pytest.raises(NeuralProviderError):
            await OpenAIProvider(http).propose_live_pilot(
                "synthetic-secret",
                "deep",
                {"allowed_symbols": ["BTC"], "max_proposal_notional": "24"},
                "a" * 64,
            )


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "change",
    [
        {"status": "incomplete"},
        {"model": "another-model"},
        {"output": [{"type": "message", "content": [{"type": "refusal", "refusal": "No"}]}]},
        {"output": [{"type": "function_call", "name": "buy", "arguments": "{}"}]},
        {"output": provider_response()["output"] * 2},
    ],
)
async def test_live_pilot_refusal_incomplete_wrong_model_and_tools_fail_closed(
    change: dict[str, Any],
) -> None:
    async with httpx.AsyncClient(
        transport=httpx.MockTransport(
            lambda request: httpx.Response(200, json=provider_response() | change)
        )
    ) as http:
        with pytest.raises(NeuralProviderError):
            await OpenAIProvider(http).propose_live_pilot(
                "synthetic-secret",
                "deep",
                {"allowed_symbols": ["BTC"], "max_proposal_notional": "24"},
                "a" * 64,
            )


def test_live_endpoint_authenticates_and_preserves_fresh_call_shape(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls = []

    class Provider:
        async def propose_live_pilot(
            self, credential: str, profile: str, context: dict[str, object], safety: str
        ) -> ShadowDecision:
            calls.append((credential, profile, copy.deepcopy(context), safety))
            return ShadowDecision(
                **decision(),
                metadata=ResponseMetadata(provider="openai", model="gpt-5.6-sol", profile="deep"),
            )

    monkeypatch.setenv("INTERNAL_SERVICE_TOKEN", "synthetic-internal-token")
    monkeypatch.setattr(main.registry, "get", lambda provider: Provider())
    client = TestClient(main.app)
    path = "/internal/neural/live-pilot-decision"
    assert client.post(path, json=pilot_request()).status_code == 401
    assert calls == []
    response = client.post(
        path, json=pilot_request(), headers={"authorization": "Bearer synthetic-internal-token"}
    )
    assert response.status_code == 200
    assert response.json()["purpose"] == "LIVE_PILOT"
    assert response.json()["decision"]["decision"] == "ABSTAIN"
    assert len(calls) == 1
    assert calls[0][0] == "synthetic-secret"
    assert calls[0][1] == "deep"
    for field in ("credential", "provider", "profile", "safety_identifier", "account_id"):
        assert field not in calls[0][2]


def test_live_endpoint_validation_and_provider_errors_are_sanitized(
    monkeypatch: pytest.MonkeyPatch,
    caplog: pytest.LogCaptureFixture,
) -> None:
    class Failed:
        async def propose_live_pilot(self, *args: object) -> ShadowDecision:
            raise NeuralProviderError(ErrorCode.TIMEOUT)

    monkeypatch.setenv("INTERNAL_SERVICE_TOKEN", "synthetic-internal-token")
    monkeypatch.setattr(main.registry, "get", lambda provider: Failed())
    client = TestClient(main.app)
    headers = {"authorization": "Bearer synthetic-internal-token"}
    path = "/internal/neural/live-pilot-decision"
    response = client.post(path, json=pilot_request(), headers=headers)
    assert response.status_code == 400
    assert response.json() == {"detail": {"code": "TIMEOUT"}}
    response = client.post(
        path, json=pilot_request() | {"account_id": "sensitive-invalid"}, headers=headers
    )
    assert response.status_code == 422
    assert response.json() == {"detail": {"code": "INVALID_REQUEST"}}
    assert "synthetic-secret" not in caplog.text
    assert "sensitive-invalid" not in caplog.text


@pytest.mark.asyncio
@pytest.mark.parametrize("status", [307, 308])
@pytest.mark.parametrize(
    "location",
    [
        "https://api.openai.com/v1/redirected",
        "https://different.example/receive",
    ],
)
async def test_live_provider_never_follows_redirects_with_injected_client(
    status: int,
    location: str,
) -> None:
    calls = []

    def respond(request: httpx.Request) -> httpx.Response:
        calls.append(str(request.url))
        if len(calls) > 1:
            return httpx.Response(200, json=provider_response())
        return httpx.Response(status, headers={"location": location}, json=provider_response())

    async with httpx.AsyncClient(
        transport=httpx.MockTransport(respond), follow_redirects=True
    ) as http:
        with pytest.raises(NeuralProviderError) as caught:
            await OpenAIProvider(http).propose_live_pilot(
                "synthetic-secret",
                "deep",
                {"allowed_symbols": ["BTC"], "max_proposal_notional": "24"},
                "a" * 64,
            )
    assert caught.value.code == ErrorCode.PROVIDER_UNAVAILABLE
    assert calls == ["https://api.openai.com/v1/responses"]
