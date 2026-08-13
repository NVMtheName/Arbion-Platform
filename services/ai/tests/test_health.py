import os

from fastapi.testclient import TestClient

from app.main import app

client = TestClient(app)


def test_health() -> None:
    response = client.get("/healthz")

    assert response.status_code == 200
    assert response.json() == {"service": "ai", "status": "ok"}


def test_internal_routes_require_service_authentication() -> None:
    os.environ["INTERNAL_SERVICE_TOKEN"] = "internal-test-token"
    response = client.post(
        "/internal/neural/verify",
        json={"provider": "openai", "credential": "secret-value"},
    )
    assert response.status_code == 401
    assert "secret-value" not in response.text
