from __future__ import annotations

from dataclasses import dataclass
from enum import StrEnum
from typing import Any


class Capability(StrEnum):
    CREDENTIAL_VERIFICATION = "credential_verification"
    MODEL_DISCOVERY = "model_discovery"
    TEXT_GENERATION = "text_generation"
    STREAMING = "streaming"
    STRUCTURED_OUTPUT = "structured_output"
    TOOL_CALLING = "tool_calling"
    VISION = "vision"
    REASONING = "reasoning"
    WEB_SEARCH = "web_search"


class CredentialType(StrEnum):
    API_KEY = "api_key"
    AUTHORIZATION_KEY = "authorization_key"
    OAUTH_TOKEN = "oauth_token"
    MANAGED_IDENTITY = "managed_identity"


class ErrorCode(StrEnum):
    AUTHENTICATION_FAILED = "AUTHENTICATION_FAILED"
    RATE_LIMITED = "RATE_LIMITED"
    PROVIDER_UNAVAILABLE = "PROVIDER_UNAVAILABLE"
    TIMEOUT = "TIMEOUT"
    INVALID_REQUEST = "INVALID_REQUEST"
    UNSUPPORTED = "UNSUPPORTED"
    INTERNAL_ERROR = "INTERNAL_ERROR"


class NeuralProviderError(Exception):
    def __init__(self, code: ErrorCode) -> None:
        self.code = code
        super().__init__(code.value)


@dataclass(frozen=True)
class ProviderModel:
    id: str
    display_name: str
    provider: str
    capabilities: tuple[Capability, ...] = ()


@dataclass(frozen=True)
class VerificationResult:
    provider: str
    valid: bool = True


@dataclass(frozen=True)
class ResponseMetadata:
    provider: str
    model: str | None = None
    input_usage: int | None = None
    output_usage: int | None = None
    request_id: str | None = None
    latency_ms: int | None = None
    usage_metadata: dict[str, Any] | None = None


@dataclass(frozen=True)
class Insight:
    summary: str
    key_points: tuple[str, ...]
    risk_flags: tuple[str, ...]
    limitations: tuple[str, ...]
    requires_current_data: bool
    metadata: ResponseMetadata
