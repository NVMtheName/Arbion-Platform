import os

import pytest
from fastapi.testclient import TestClient
from pydantic import ValidationError

from app.main import (
    ShadowDecisionRequest,
    ShadowMarketFact,
    ShadowPositionFact,
    ShadowRecentDecision,
    app,
    validate_production_configuration,
)

client = TestClient(app)


def test_shadow_position_performance_requires_complete_exact_provenance() -> None:
    position = ShadowPositionFact.model_validate(
        {
            "symbol": "SPY",
            "instrument": "EQUITY",
            "quantity": "2",
            "available_quantity": "2",
            "market_value_usd": "1050",
            "performance_status": "AVAILABLE",
            "average_price_usd": "500",
            "current_price_usd": "525",
            "day_profit_loss_usd": "0",
            "day_profit_loss_percent": "0",
            "open_profit_loss_usd": "50",
            "open_profit_loss_percent": "5",
            "price_basis": "PROVIDER_POSITION_MARKET_VALUE_PER_UNIT",
        }
    )
    assert position.day_profit_loss_usd == "0"

    payload = position.model_dump()
    payload["performance_status"] = "UNAVAILABLE"
    with pytest.raises(ValidationError, match="unavailable performance"):
        ShadowPositionFact.model_validate(payload)

    payload = position.model_dump()
    payload["day_profit_loss_percent"] = ""
    with pytest.raises(ValidationError, match="day performance requires both"):
        ShadowPositionFact.model_validate(payload)

    payload = position.model_dump()
    payload["price_basis"] = ""
    with pytest.raises(ValidationError, match="exact provider price provenance"):
        ShadowPositionFact.model_validate(payload)


def test_paper_position_accepts_exact_market_reference_provenance() -> None:
    position = ShadowPositionFact.model_validate(
        {
            "symbol": "BTC",
            "instrument": "CRYPTO",
            "quantity": "0.0003177277",
            "available_quantity": "0.0003177277",
            "market_value_usd": "24.8139964102",
            "performance_status": "PARTIAL",
            "average_price_usd": "78683.7037126445",
            "current_price_usd": "78098.31",
            "open_profit_loss_usd": "-0.1859957979",
            "open_profit_loss_percent": "-0.7439842615",
            "price_basis": "PROVIDER_MARKET_REFERENCE",
        }
    )

    assert position.price_basis == "PROVIDER_MARKET_REFERENCE"
    assert position.performance_status == "PARTIAL"


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


def equity_shadow_request_payload() -> dict[str, object]:
    return {
        "provider": "openai",
        "credential": "secret-value",
        "profile": "deep",
        "objective": "Preserve capital.",
        "allowed_symbols": ["AAPL"],
        "max_proposal_notional": "1",
        "available_cash_usd": "100",
        "buying_power_usd": "100",
        "positions": [],
        "markets": [
            {
                "symbol": "AAPL",
                "asset_class": "EQUITY",
                "currency": "USD",
                "bid": "199.9",
                "ask": "200.1",
                "mark": "",
                "last": "",
                "change_percent_1h": "",
                "change_percent_6h": "",
                "change_percent_24h": "",
                "volume_24h": "",
                "feed": "schwab_market_data",
                "quality": "BROKER_REALTIME",
                "observed_at": "2026-08-25T14:00:00Z",
                "history_status": "UNAVAILABLE",
                "history_granularity_seconds": 0,
                "history_contiguous_intervals": 0,
                "history_expected_intervals": 0,
                "history_feed": "",
                "history_quality": "",
                "liquidity_status": "UNAVAILABLE",
            }
        ],
        "market_event_coverage": [
            {
                "symbol": "AAPL",
                "status": "AVAILABLE",
                "lookback_days": 30,
                "event_count": 1,
                "resolver_provider": "sec_edgar",
                "resolver_feed": "company_tickers",
                "resolver_quality": "AGGREGATED_REFERENCE",
                "resolver_received_at": "2026-08-25T14:00:01Z",
            }
        ],
        "market_events": [
            {
                "symbol": "AAPL",
                "event_type": "SEC_OWNERSHIP_FILING",
                "form": "4",
                "is_amendment": False,
                "evidence_id": "0000320193-26-000001",
                "issuer_cik": "0000320193",
                "occurred_at": "2026-08-24T14:00:00Z",
                "provider": "sec_edgar",
                "feed": "submissions",
                "quality": "FILING",
            }
        ],
        "recent_decisions": [],
        "observed_at": "2026-08-25T14:00:00Z",
        "safety_identifier": "a" * 64,
    }


def test_shadow_request_accepts_bounded_primary_filing_event_identity() -> None:
    request = ShadowDecisionRequest.model_validate(equity_shadow_request_payload())
    assert request.market_event_coverage[0].event_count == 1
    assert request.market_events[0].form == "4"

    payload = equity_shadow_request_payload()
    coverage = payload["market_event_coverage"]
    assert isinstance(coverage, list) and isinstance(coverage[0], dict)
    coverage[0]["event_count"] = 0
    payload["market_events"] = []
    request = ShadowDecisionRequest.model_validate(payload)
    assert request.market_event_coverage[0].status == "AVAILABLE"
    assert request.market_events == []


def test_shadow_request_rejects_event_count_or_amendment_inconsistency() -> None:
    payload = equity_shadow_request_payload()
    coverage = payload["market_event_coverage"]
    assert isinstance(coverage, list) and isinstance(coverage[0], dict)
    coverage[0]["event_count"] = 0
    with pytest.raises(ValidationError, match="event coverage count does not match"):
        ShadowDecisionRequest.model_validate(payload)

    payload = equity_shadow_request_payload()
    events = payload["market_events"]
    assert isinstance(events, list) and isinstance(events[0], dict)
    events[0]["is_amendment"] = True
    with pytest.raises(ValidationError, match="filing amendment metadata is inconsistent"):
        ShadowDecisionRequest.model_validate(payload)

    payload = equity_shadow_request_payload()
    coverage = payload["market_event_coverage"]
    assert isinstance(coverage, list) and isinstance(coverage[0], dict)
    coverage[0]["resolver_feed"] = "untrusted_feed"
    with pytest.raises(ValidationError, match="exact SEC resolver provenance"):
        ShadowDecisionRequest.model_validate(payload)


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


@pytest.mark.parametrize("disposition", ["SIMULATED_FILLED", "SIMULATED_REJECTED"])
def test_shadow_request_accepts_bounded_paper_decision_memory(
    disposition: str,
) -> None:
    payload = equity_shadow_request_payload()
    payload["allowed_symbols"] = ["BTC"]
    payload["markets"] = [
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
            "history_observed_at": None,
        }
    ]
    payload["market_event_coverage"] = []
    payload["market_events"] = []
    payload["recent_decisions"] = [
        {
            "decision": "PROPOSE",
            "symbol": "BTC",
            "side": "BUY",
            "disposition": disposition,
            "occurred_at": "2026-08-25T13:30:00Z",
        }
    ]

    request = ShadowDecisionRequest.model_validate(payload)

    assert request.recent_decisions[0].disposition == disposition


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
