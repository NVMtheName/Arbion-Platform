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
