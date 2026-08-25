import os

import pytest
from fastapi.testclient import TestClient

from app.main import app, validate_production_configuration

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
                    "change_percent_24h": "",
                    "volume_24h": "",
                    "feed": "exchange",
                    "quality": "REAL_TIME_SINGLE_VENUE",
                    "observed_at": "2026-08-25T14:00:00Z",
                }
            ],
            "observed_at": "2026-08-25T14:00:00Z",
            "safety_identifier": "a" * 64,
        },
    )
    assert shadow_response.status_code == 401
    assert "secret-value" not in shadow_response.text


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
