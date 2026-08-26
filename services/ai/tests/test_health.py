import os

import pytest
from fastapi.testclient import TestClient
from pydantic import ValidationError

from app.main import (
    ShadowDecisionRequest,
    ShadowMarketFact,
    ShadowRecentDecision,
    app,
    validate_production_configuration,
)

client = TestClient(app)


def test_health() -> None:
    response = client.get("/healthz")

    assert response.status_code == 200
    assert response.json() == {"service": "ai", "status": "ok"}


def test_readiness() -> None:
    response = client.get("/readyz")
    assert response.status_code == 200
    assert response.json() == {"service": "ai", "status": "ready"}


def test_production_requires_strong_internal_token(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("ARBION_ENV", "production")
    monkeypatch.setenv("INTERNAL_SERVICE_TOKEN", "local-internal-development-token")
    with pytest.raises(RuntimeError, match="internal service authentication"):
        validate_production_configuration()


def test_internal_routes_require_service_authentication() -> None:
    os.environ["INTERNAL_SERVICE_TOKEN"] = "internal-test-token"
    response = client.post(
        "/internal/neural/verify",
        json={"provider": "openai", "credential": "secret-value"},
    )
    assert response.status_code == 401
    assert "secret-value" not in response.text

    insight_response = client.post(
        "/internal/neural/insight",
        json={
            "provider": "openai",
            "credential": "secret-value",
            "profile": "fast",
            "prompt": "question",
            "safety_identifier": "a" * 64,
        },
    )
    assert insight_response.status_code == 401
    assert "secret-value" not in insight_response.text

    proposal_response = client.post(
        "/internal/neural/trade-proposal",
        json={
            "provider": "openai",
            "credential": "secret-value",
            "profile": "core",
            "objective": "Keep exposure small.",
            "symbol": "BTC",
            "side": "BUY",
            "max_size": "50",
            "max_size_unit": "USD",
            "available_cash": "200",
            "position_quantity": "0.012",
            "position_available_quantity": "0.01",
            "observed_at": "2026-08-24T12:00:00Z",
            "safety_identifier": "a" * 64,
        },
    )
    assert proposal_response.status_code == 401
    assert "secret-value" not in proposal_response.text

    shadow_response = client.post(
        "/internal/neural/shadow-decision",
        json={
            "provider": "openai",
            "credential": "secret-value",
            "profile": "deep",
            "objective": "Preserve capital.",
            "allowed_symbols": ["BTC"],
            "max_proposal_notional": "1",
            "available_cash_usd": "100",
            "buying_power_usd": "100",
            "positions": [],
            "markets": [
                {
                    "symbol": "BTC",
                    "asset_class": "CRYPTO",
                    "currency": "USD",
                    "bid": "99",
                    "ask": "101",
                    "mark": "100",
                    "last": "100",
                    "change_percent_1h": "",
                    "change_percent_6h": "",
                    "change_percent_24h": "",
                    "volume_24h": "",
                    "feed": "exchange",
                    "quality": "REAL_TIME_SINGLE_VENUE",
                    "observed_at": "2026-08-25T14:00:00Z",
                    "history_status": "UNAVAILABLE",
                    "history_granularity_seconds": 900,
                    "history_contiguous_intervals": 0,
                    "history_expected_intervals": 96,
                    "history_feed": "",
                    "history_quality": "",
                }
            ],
            "observed_at": "2026-08-25T14:00:00Z",
            "safety_identifier": "a" * 64,
        },
    )
    assert shadow_response.status_code == 401
    assert "secret-value" not in shadow_response.text


def test_shadow_market_history_rejects_claimed_complete_gaps() -> None:
    with pytest.raises(ValidationError, match="complete history requires every expected interval"):
        ShadowMarketFact.model_validate(
            {
                "symbol": "BTC",
                "asset_class": "CRYPTO",
                "currency": "USD",
                "bid": "99",
                "ask": "101",
                "mark": "100",
                "last": "100",
                "change_percent_1h": "1",
                "change_percent_6h": "2",
                "change_percent_24h": "3",
                "volume_24h": "1000",
                "feed": "rest_ticker",
                "quality": "REAL_TIME_SINGLE_VENUE",
                "observed_at": "2026-08-25T14:00:00Z",
                "history_status": "COMPLETE",
                "history_granularity_seconds": 900,
                "history_contiguous_intervals": 95,
                "history_expected_intervals": 96,
                "history_feed": "rest_candles",
                "history_quality": "REAL_TIME_SINGLE_VENUE",
                "history_observed_at": "2026-08-25T14:00:00Z",
            }
        )


def test_shadow_market_accepts_complete_bounded_liquidity_provenance() -> None:
    market = ShadowMarketFact.model_validate(
        {
            "symbol": "BTC",
            "asset_class": "CRYPTO",
            "currency": "USD",
            "bid": "99",
            "ask": "101",
            "mark": "100",
            "last": "100",
            "change_percent_1h": "",
            "change_percent_6h": "",
            "change_percent_24h": "",
            "volume_24h": "",
            "feed": "rest_ticker",
            "quality": "REAL_TIME_SINGLE_VENUE",
            "observed_at": "2026-08-25T14:00:00Z",
            "history_status": "UNAVAILABLE",
            "history_granularity_seconds": 900,
            "history_contiguous_intervals": 0,
            "history_expected_intervals": 96,
            "history_feed": "",
            "history_quality": "",
            "liquidity_status": "AVAILABLE",
            "spread_bps": "2.5",
            "bid_depth_usd": "5000",
            "ask_depth_usd": "4800",
            "bid_levels": 10,
            "ask_levels": 10,
            "liquidity_feed": "advanced_trade_public_book",
            "liquidity_quality": "REAL_TIME_SINGLE_VENUE",
            "liquidity_observed_at": "2026-08-25T14:00:01Z",
        }
    )
    assert market.liquidity_status == "AVAILABLE"
    assert market.bid_depth_usd == "5000"


def test_shadow_market_rejects_unavailable_liquidity_with_derived_values() -> None:
    with pytest.raises(ValidationError, match="unusable liquidity cannot contain"):
        ShadowMarketFact.model_validate(
            {
                "symbol": "BTC",
                "asset_class": "CRYPTO",
                "currency": "USD",
                "bid": "99",
                "ask": "101",
                "mark": "100",
                "last": "100",
                "change_percent_1h": "",
                "change_percent_6h": "",
                "change_percent_24h": "",
                "volume_24h": "",
                "feed": "rest_ticker",
                "quality": "REAL_TIME_SINGLE_VENUE",
                "observed_at": "2026-08-25T14:00:00Z",
                "history_status": "UNAVAILABLE",
                "history_granularity_seconds": 900,
                "history_contiguous_intervals": 0,
                "history_expected_intervals": 96,
                "history_feed": "",
                "history_quality": "",
                "liquidity_status": "UNAVAILABLE",
                "spread_bps": "2.5",
            }
        )


def test_shadow_recent_decision_rejects_inconsistent_abstention() -> None:
    with pytest.raises(ValidationError, match="abstention memory is inconsistent"):
        ShadowRecentDecision.model_validate(
            {
                "decision": "ABSTAIN",
                "symbol": "BTC",
                "side": "NONE",
                "disposition": "ABSTAINED",
                "occurred_at": "2026-08-25T13:30:00Z",
            }
        )


def test_shadow_request_rejects_decision_memory_older_than_six_hours() -> None:
    with pytest.raises(ValidationError, match="outside the bounded window"):
        ShadowDecisionRequest.model_validate(
            {
                "provider": "openai",
                "credential": "secret-value",
                "profile": "deep",
                "objective": "Preserve capital.",
                "allowed_symbols": ["BTC"],
                "max_proposal_notional": "1",
                "available_cash_usd": "100",
                "buying_power_usd": "100",
                "positions": [],
                "markets": [
                    {
                        "symbol": "BTC",
                        "asset_class": "CRYPTO",
                        "currency": "USD",
                        "bid": "99",
                        "ask": "101",
                        "mark": "100",
                        "last": "100",
                        "change_percent_1h": "",
                        "change_percent_6h": "",
                        "change_percent_24h": "",
                        "volume_24h": "",
                        "feed": "exchange",
                        "quality": "REAL_TIME_SINGLE_VENUE",
                        "observed_at": "2026-08-25T14:00:00Z",
                        "history_status": "UNAVAILABLE",
                        "history_granularity_seconds": 900,
                        "history_contiguous_intervals": 0,
                        "history_expected_intervals": 96,
                        "history_feed": "",
                        "history_quality": "",
                    }
                ],
                "recent_decisions": [
                    {
                        "decision": "ABSTAIN",
                        "symbol": "NONE",
                        "side": "NONE",
                        "disposition": "ABSTAINED",
                        "occurred_at": "2026-08-25T07:59:59Z",
                    }
                ],
                "observed_at": "2026-08-25T14:00:00Z",
                "safety_identifier": "a" * 64,
            }
        )


def test_internal_insight_rejects_unknown_profile_before_provider_call() -> None:
    os.environ["INTERNAL_SERVICE_TOKEN"] = "internal-test-token"
    response = client.post(
        "/internal/neural/insight",
        headers={"authorization": "Bearer internal-test-token"},
        json={
            "provider": "openai",
            "credential": "secret-value",
            "profile": "unbounded",
            "prompt": "question",
            "safety_identifier": "a" * 64,
        },
    )
    assert response.status_code == 422
    assert "secret-value" not in response.text
