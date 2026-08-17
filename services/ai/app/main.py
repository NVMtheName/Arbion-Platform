import os
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Literal

from fastapi import FastAPI, Header, HTTPException
from pydantic import BaseModel, Field

from .neural.models import NeuralProviderError
from .neural.registry import default_registry


def validate_production_configuration() -> None:
    if os.environ.get("ARBION_ENV") != "production":
        return
    token = os.environ.get("INTERNAL_SERVICE_TOKEN", "")
    if len(token) < 32 or token == "local-internal-development-token":
        raise RuntimeError("production internal service authentication is not configured")


@asynccontextmanager
async def lifespan(_: FastAPI) -> AsyncIterator[None]:
    validate_production_configuration()
    yield


app = FastAPI(title="Arbion AI", version="0.1.0", lifespan=lifespan)
registry = default_registry()


class ProviderRequest(BaseModel):
    provider: Literal["openai", "anthropic", "gemini"]
    credential: str = Field(min_length=8, max_length=4096)


class InsightRequest(ProviderRequest):
    profile: Literal["fast", "core", "deep"] = "fast"
    prompt: str = Field(min_length=1, max_length=2000)
    safety_identifier: str = Field(min_length=64, max_length=64, pattern=r"^[a-f0-9]{64}$")


def authorize(value: str | None) -> None:
    expected = os.environ.get("INTERNAL_SERVICE_TOKEN", "")
    if not expected or value != f"Bearer {expected}":
        raise HTTPException(status_code=401, detail="internal authentication required")


@app.post("/internal/neural/verify")
async def verify(
    request: ProviderRequest, authorization: str | None = Header(None)
) -> dict[str, object]:
    authorize(authorization)
    try:
        result = await registry.get(request.provider).verify_credentials(request.credential)
        return {"valid": result.valid, "provider": result.provider}
    except NeuralProviderError as exc:
        raise HTTPException(status_code=400, detail={"code": exc.code.value}) from None


@app.post("/internal/neural/models")
async def models(
    request: ProviderRequest, authorization: str | None = Header(None)
) -> dict[str, object]:
    authorize(authorization)
    try:
        values = await registry.get(request.provider).list_models(request.credential)
        return {
            "models": [
                {
                    "id": value.id,
                    "display_name": value.display_name,
                    "provider": value.provider,
                    "capabilities": list(value.capabilities),
                }
                for value in values
            ]
        }
    except NeuralProviderError as exc:
        raise HTTPException(status_code=400, detail={"code": exc.code.value}) from None


@app.post("/internal/neural/insight")
async def insight(
    request: InsightRequest, authorization: str | None = Header(None)
) -> dict[str, object]:
    authorize(authorization)
    try:
        result = await registry.get(request.provider).analyze(
            request.credential,
            request.profile,
            request.prompt,
            request.safety_identifier,
        )
        return {
            "insight": {
                "summary": result.summary,
                "key_points": list(result.key_points),
                "risk_flags": list(result.risk_flags),
                "limitations": list(result.limitations),
                "requires_current_data": result.requires_current_data,
                "metadata": {
                    "provider": result.metadata.provider,
                    "model": result.metadata.model,
                    "profile": result.metadata.profile,
                    "input_usage": result.metadata.input_usage,
                    "output_usage": result.metadata.output_usage,
                    "request_id": result.metadata.request_id,
                    "latency_ms": result.metadata.latency_ms,
                },
            }
        }
    except NeuralProviderError as exc:
        raise HTTPException(status_code=400, detail={"code": exc.code.value}) from None


@app.get("/healthz")
def health() -> dict[str, str]:
    """Report process liveness without checking downstream dependencies."""
    return {"service": "ai", "status": "ok"}


@app.get("/readyz")
def ready() -> dict[str, str]:
    """Report readiness after startup configuration validation has succeeded."""
    return {"service": "ai", "status": "ready"}
