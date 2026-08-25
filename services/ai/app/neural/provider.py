from __future__ import annotations

from abc import ABC, abstractmethod
from collections.abc import AsyncIterator
from typing import Any

from .models import (
    Capability,
    CredentialType,
    ErrorCode,
    Insight,
    NeuralProviderError,
    ProviderModel,
    ShadowDecision,
    TradeProposal,
    VerificationResult,
)


class NeuralProvider(ABC):
    id: str
    credential_types: tuple[CredentialType, ...]

    @property
    @abstractmethod
    def capabilities(self) -> frozenset[Capability]: ...

    @abstractmethod
    async def verify_credentials(self, credential: str) -> VerificationResult: ...

    @abstractmethod
    async def list_models(self, credential: str) -> list[ProviderModel]: ...

    async def analyze(
        self,
        credential: str,
        profile: str,
        prompt: str,
        safety_identifier: str,
    ) -> Insight:
        raise NeuralProviderError(ErrorCode.UNSUPPORTED)

    async def propose_trade(
        self,
        credential: str,
        profile: str,
        context: dict[str, str],
        safety_identifier: str,
    ) -> TradeProposal:
        raise NeuralProviderError(ErrorCode.UNSUPPORTED)

    async def propose_shadow(
        self,
        credential: str,
        profile: str,
        context: dict[str, object],
        safety_identifier: str,
    ) -> ShadowDecision:
        raise NeuralProviderError(ErrorCode.UNSUPPORTED)

    async def generate(self, credential: str, model: str, request: Any) -> Any:
        raise NotImplementedError

    async def structured_output(self, credential: str, model: str, request: Any) -> Any:
        raise NotImplementedError

    async def stream(self, credential: str, model: str, request: Any) -> AsyncIterator[Any]:
        raise NotImplementedError
        yield

    def tool_support(self, model: str) -> bool:
        return Capability.TOOL_CALLING in self.capabilities
