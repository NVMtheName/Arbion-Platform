import os
import re
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from datetime import datetime, timedelta
from decimal import Decimal, InvalidOperation
from typing import Literal, Self

from fastapi import FastAPI, Header, HTTPException
from pydantic import BaseModel, Field, model_validator

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


class TradeProposalRequest(ProviderRequest):
    profile: Literal["core"] = "core"
    objective: str = Field(min_length=1, max_length=500)
    symbol: str = Field(min_length=1, max_length=16, pattern=r"^[A-Z][A-Z0-9]{0,15}$")
    side: Literal["BUY", "SELL"]
    max_size: str = Field(pattern=r"^(0|[1-9][0-9]{0,17})(\.[0-9]{1,18})?$")
    max_size_unit: str = Field(min_length=1, max_length=16, pattern=r"^[A-Z][A-Z0-9]{0,15}$")
    available_cash: str = Field(pattern=r"^(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$")
    position_quantity: str = Field(pattern=r"^(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$")
    position_available_quantity: str = Field(pattern=r"^(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$")
    observed_at: datetime
    safety_identifier: str = Field(min_length=64, max_length=64, pattern=r"^[a-f0-9]{64}$")


class ShadowPositionFact(BaseModel):
    symbol: str = Field(min_length=1, max_length=16, pattern=r"^[A-Z][A-Z0-9.-]{0,15}$")
    instrument: Literal["EQUITY", "CRYPTO"]
    quantity: str = Field(pattern=r"^-?(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$")
    available_quantity: str = Field(pattern=r"^(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$")
    market_value_usd: str = Field(pattern=r"^(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$")
    performance_status: Literal["AVAILABLE", "PARTIAL", "UNAVAILABLE"]
    average_price_usd: str = Field(default="", pattern=r"^(|0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$")
    current_price_usd: str = Field(default="", pattern=r"^(|0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$")
    day_profit_loss_usd: str = Field(
        default="", pattern=r"^(|-?(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?)$"
    )
    day_profit_loss_percent: str = Field(
        default="", pattern=r"^(|-?(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?)$"
    )
    open_profit_loss_usd: str = Field(
        default="", pattern=r"^(|-?(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?)$"
    )
    open_profit_loss_percent: str = Field(
        default="", pattern=r"^(|-?(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?)$"
    )
    price_basis: Literal[
        "",
        "PROVIDER_POSITION_MARKET_VALUE_PER_UNIT",
        "PROVIDER_MARKET_REFERENCE",
    ] = ""

    @model_validator(mode="after")
    def validate_performance_consistency(self) -> Self:
        if self.average_price_usd and Decimal(self.average_price_usd) <= 0:
            raise ValueError("average price must be positive when available")
        if self.current_price_usd and Decimal(self.current_price_usd) <= 0:
            raise ValueError("current price must be positive when available")
        if bool(self.day_profit_loss_usd) != bool(self.day_profit_loss_percent):
            raise ValueError("day performance requires both amount and percentage")
        if bool(self.open_profit_loss_usd) != bool(self.open_profit_loss_percent):
            raise ValueError("open performance requires both amount and percentage")
        if bool(self.current_price_usd) != bool(self.price_basis):
            raise ValueError("current price requires exact provider price provenance")
        supplied = (
            bool(self.average_price_usd),
            bool(self.current_price_usd),
            bool(self.day_profit_loss_usd),
            bool(self.open_profit_loss_usd),
        )
        complete = all(supplied)
        if self.performance_status == "UNAVAILABLE" and any(supplied):
            raise ValueError("unavailable performance cannot contain derived facts")
        if self.performance_status == "PARTIAL" and (not any(supplied) or complete):
            raise ValueError("partial performance must contain an incomplete fact set")
        if self.performance_status == "AVAILABLE" and not complete:
            raise ValueError("available performance requires every normalized fact")
        return self


class ShadowMarketFact(BaseModel):
    symbol: str = Field(min_length=1, max_length=16, pattern=r"^[A-Z][A-Z0-9.-]{0,15}$")
    asset_class: Literal["EQUITY", "CRYPTO"]
    currency: Literal["USD"]
    bid: str = Field(pattern=r"^(|0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$")
    ask: str = Field(pattern=r"^(|0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$")
    mark: str = Field(pattern=r"^(|0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$")
    last: str = Field(pattern=r"^(|0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$")
    change_percent_1h: str = Field(pattern=r"^(|-?(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?)$")
    change_percent_6h: str = Field(pattern=r"^(|-?(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?)$")
    change_percent_24h: str = Field(pattern=r"^(|-?(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?)$")
    volume_24h: str = Field(pattern=r"^(|0|[1-9][0-9]{0,29})(\.[0-9]{1,18})?$")
    feed: str = Field(min_length=1, max_length=64)
    quality: str = Field(min_length=1, max_length=64)
    observed_at: datetime
    history_status: Literal["COMPLETE", "PARTIAL", "STALE", "UNAVAILABLE"]
    history_granularity_seconds: int = Field(ge=0, le=86400)
    history_contiguous_intervals: int = Field(ge=0, le=300)
    history_expected_intervals: int = Field(ge=0, le=300)
    history_feed: str = Field(max_length=64)
    history_quality: str = Field(max_length=64)
    history_observed_at: datetime | None = None
    liquidity_status: Literal["AVAILABLE", "STALE", "UNAVAILABLE"] = "UNAVAILABLE"
    spread_bps: str = Field(default="", pattern=r"^(|0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$")
    bid_depth_usd: str = Field(default="", pattern=r"^(|0|[1-9][0-9]{0,29})(\.[0-9]{1,18})?$")
    ask_depth_usd: str = Field(default="", pattern=r"^(|0|[1-9][0-9]{0,29})(\.[0-9]{1,18})?$")
    bid_levels: int = Field(default=0, ge=0, le=10)
    ask_levels: int = Field(default=0, ge=0, le=10)
    liquidity_feed: str = Field(default="", max_length=64)
    liquidity_quality: str = Field(default="", max_length=64)
    liquidity_observed_at: datetime | None = None

    @model_validator(mode="after")
    def validate_history_consistency(self) -> Self:
        derived = bool(self.change_percent_1h or self.change_percent_6h)
        if self.history_status in {"STALE", "UNAVAILABLE"}:
            if derived:
                raise ValueError("unusable history cannot contain derived changes")
            return self
        if (
            self.history_granularity_seconds <= 0
            or self.history_expected_intervals <= 0
            or self.history_contiguous_intervals < 4
            or not self.change_percent_1h
            or not self.history_feed
            or not self.history_quality
            or self.history_observed_at is None
        ):
            raise ValueError("usable history requires complete provenance and a 1-hour window")
        if self.history_contiguous_intervals >= 24 and not self.change_percent_6h:
            raise ValueError("a covered 6-hour window requires its derived change")
        if self.history_contiguous_intervals < 24 and self.change_percent_6h:
            raise ValueError("a 6-hour change requires 24 contiguous intervals")
        if self.history_status == "COMPLETE" and (
            self.history_contiguous_intervals < self.history_expected_intervals
        ):
            raise ValueError("complete history requires every expected interval")
        if self.history_status == "PARTIAL" and (
            self.history_contiguous_intervals >= self.history_expected_intervals
        ):
            raise ValueError("fully covered history cannot be labeled partial")
        return self

    @model_validator(mode="after")
    def validate_liquidity_consistency(self) -> Self:
        values = (self.spread_bps, self.bid_depth_usd, self.ask_depth_usd)
        if self.liquidity_status == "AVAILABLE":
            if (
                not all(values)
                or any(Decimal(value) <= 0 for value in values)
                or self.bid_levels <= 0
                or self.ask_levels <= 0
                or not self.liquidity_feed
                or not self.liquidity_quality
                or self.liquidity_observed_at is None
            ):
                raise ValueError("available liquidity requires bounded values and provenance")
            return self
        if any(values) or self.bid_levels or self.ask_levels:
            raise ValueError("unusable liquidity cannot contain book-derived values")
        if self.liquidity_status == "STALE" and (
            not self.liquidity_feed
            or not self.liquidity_quality
            or self.liquidity_observed_at is None
        ):
            raise ValueError("stale liquidity requires provenance")
        if self.liquidity_status == "UNAVAILABLE" and (
            self.liquidity_feed or self.liquidity_quality or self.liquidity_observed_at is not None
        ):
            raise ValueError("unavailable liquidity cannot claim provenance")
        return self


class ShadowRecentDecision(BaseModel):
    decision: Literal["ABSTAIN", "PROPOSE"]
    symbol: str = Field(min_length=1, max_length=16, pattern=r"^[A-Z][A-Z0-9.-]{0,15}$")
    side: Literal["BUY", "SELL", "NONE"]
    disposition: Literal["ABSTAINED", "WOULD_HAVE_SUBMITTED", "HELD_BY_CONTROLS"]
    occurred_at: datetime

    @model_validator(mode="after")
    def validate_decision_consistency(self) -> Self:
        if self.decision == "ABSTAIN":
            if self.symbol != "NONE" or self.side != "NONE" or self.disposition != "ABSTAINED":
                raise ValueError("abstention memory is inconsistent")
        elif self.side == "NONE" or self.disposition == "ABSTAINED":
            raise ValueError("proposal memory is inconsistent")
        return self


class ShadowMarketEventCoverage(BaseModel):
    symbol: str = Field(min_length=1, max_length=16, pattern=r"^[A-Z][A-Z0-9.-]{0,15}$")
    status: Literal["AVAILABLE", "UNAVAILABLE"]
    lookback_days: Literal[30]
    event_count: int = Field(ge=0, le=2)
    resolver_provider: str = Field(default="", max_length=64)
    resolver_feed: str = Field(default="", max_length=64)
    resolver_quality: str = Field(default="", max_length=64)
    resolver_received_at: datetime | None = None

    @model_validator(mode="after")
    def validate_coverage_consistency(self) -> Self:
        provenance = (self.resolver_provider, self.resolver_feed, self.resolver_quality)
        if self.status == "AVAILABLE":
            if not all(provenance) or self.resolver_received_at is None:
                raise ValueError("available event coverage requires resolver provenance")
            if provenance != ("sec_edgar", "company_tickers", "AGGREGATED_REFERENCE"):
                raise ValueError("event coverage requires exact SEC resolver provenance")
            if self.resolver_received_at.tzinfo is None:
                raise ValueError("resolver timestamp must include a timezone")
            return self
        if any(provenance) or self.resolver_received_at is not None or self.event_count != 0:
            raise ValueError("unavailable event coverage cannot claim evidence")
        return self


class ShadowMarketEventFact(BaseModel):
    symbol: str = Field(min_length=1, max_length=16, pattern=r"^[A-Z][A-Z0-9.-]{0,15}$")
    event_type: Literal["SEC_OWNERSHIP_FILING"]
    form: Literal["3", "3/A", "4", "4/A", "5", "5/A"]
    is_amendment: bool
    evidence_id: str = Field(pattern=r"^[0-9]{10}-[0-9]{2}-[0-9]{6}$")
    issuer_cik: str = Field(pattern=r"^[0-9]{10}$")
    occurred_at: datetime
    provider: Literal["sec_edgar"]
    feed: Literal["submissions"]
    quality: Literal["FILING"]

    @model_validator(mode="after")
    def validate_event_consistency(self) -> Self:
        if self.is_amendment != self.form.endswith("/A"):
            raise ValueError("filing amendment metadata is inconsistent")
        if self.occurred_at.tzinfo is None:
            raise ValueError("filing timestamp must include a timezone")
        return self


class ShadowDecisionRequest(ProviderRequest):
    profile: Literal["fast", "core", "deep"]
    objective: str = Field(min_length=1, max_length=500)
    allowed_symbols: list[str] = Field(min_length=1, max_length=8)
    max_proposal_notional: str = Field(pattern=r"^(0|[1-9][0-9]{0,17})(\.[0-9]{1,18})?$")
    available_cash_usd: str = Field(pattern=r"^(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$")
    buying_power_usd: str = Field(pattern=r"^(0|[1-9][0-9]{0,19})(\.[0-9]{1,18})?$")
    positions: list[ShadowPositionFact] = Field(max_length=200)
    markets: list[ShadowMarketFact] = Field(min_length=1, max_length=8)
    market_event_coverage: list[ShadowMarketEventCoverage] = Field(
        default_factory=list, max_length=8
    )
    market_events: list[ShadowMarketEventFact] = Field(default_factory=list, max_length=16)
    recent_decisions: list[ShadowRecentDecision] = Field(default_factory=list, max_length=6)
    observed_at: datetime
    safety_identifier: str = Field(min_length=64, max_length=64, pattern=r"^[a-f0-9]{64}$")

    @model_validator(mode="after")
    def validate_bounded_context(self) -> Self:
        if self.observed_at.tzinfo is None or any(
            decision.occurred_at.tzinfo is None
            or decision.occurred_at >= self.observed_at
            or self.observed_at - decision.occurred_at > timedelta(hours=6)
            for decision in self.recent_decisions
        ):
            raise ValueError("recent decision timestamps are outside the bounded window")
        coverage_symbols = [item.symbol for item in self.market_event_coverage]
        equity_symbols = {
            market.symbol for market in self.markets if market.asset_class == "EQUITY"
        }
        if (
            len(coverage_symbols) != len(set(coverage_symbols))
            or set(coverage_symbols) != equity_symbols
        ):
            raise ValueError("event coverage must match each equity market exactly")
        coverage = {item.symbol: item for item in self.market_event_coverage}
        event_counts = {symbol: 0 for symbol in coverage}
        for event in self.market_events:
            item = coverage.get(event.symbol)
            if (
                event.symbol not in self.allowed_symbols
                or item is None
                or item.status != "AVAILABLE"
                or event.occurred_at > self.observed_at + timedelta(minutes=1)
                or self.observed_at - event.occurred_at > timedelta(days=item.lookback_days)
            ):
                raise ValueError("market event is outside its bounded coverage")
            event_counts[event.symbol] += 1
        if any(
            item.event_count != event_counts[item.symbol] for item in self.market_event_coverage
        ):
            raise ValueError("event coverage count does not match supplied events")
        if any(
            item.resolver_received_at is not None
            and item.resolver_received_at > self.observed_at + timedelta(minutes=1)
            for item in self.market_event_coverage
        ):
            raise ValueError("resolver receipt is future-dated")
        return self


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


@app.post("/internal/neural/trade-proposal")
async def trade_proposal(
    request: TradeProposalRequest, authorization: str | None = Header(None)
) -> dict[str, object]:
    authorize(authorization)
    if request.observed_at.tzinfo is None:
        raise HTTPException(status_code=422, detail="observed_at must include a timezone")
    if request.max_size_unit != ("USD" if request.side == "BUY" else request.symbol):
        raise HTTPException(status_code=422, detail="max_size_unit does not match side")
    try:
        if Decimal(request.max_size) <= 0:
            raise HTTPException(status_code=422, detail="max_size must be positive")
    except InvalidOperation:
        raise HTTPException(status_code=422, detail="max_size must be a decimal") from None
    context = {
        "objective": request.objective.strip(),
        "symbol": request.symbol,
        "side": request.side,
        "max_size": request.max_size,
        "max_size_unit": request.max_size_unit,
        "available_cash_usd": request.available_cash,
        "position_quantity": request.position_quantity,
        "position_available_quantity": request.position_available_quantity,
        "observed_at": request.observed_at.isoformat(),
    }
    try:
        result = await registry.get(request.provider).propose_trade(
            request.credential,
            request.profile,
            context,
            request.safety_identifier,
        )
        return {
            "proposal": {
                "decision": result.decision,
                "requested_size": result.requested_size,
                "confidence": result.confidence,
                "thesis": result.thesis,
                "risk_flags": list(result.risk_flags),
                "limitations": list(result.limitations),
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


@app.post("/internal/neural/shadow-decision")
async def shadow_decision(
    request: ShadowDecisionRequest, authorization: str | None = Header(None)
) -> dict[str, object]:
    authorize(authorization)
    allowed = [symbol.strip().upper() for symbol in request.allowed_symbols]
    if len(set(allowed)) != len(allowed) or any(
        not re.fullmatch(r"[A-Z][A-Z0-9.-]{0,15}", symbol) for symbol in allowed
    ):
        raise HTTPException(status_code=422, detail="allowed_symbols are invalid")
    if (
        request.observed_at.tzinfo is None
        or any(
            market.observed_at.tzinfo is None
            or (
                market.history_observed_at is not None and market.history_observed_at.tzinfo is None
            )
            or (
                market.liquidity_observed_at is not None
                and market.liquidity_observed_at.tzinfo is None
            )
            for market in request.markets
        )
        or any(
            decision.occurred_at.tzinfo is None
            or decision.occurred_at >= request.observed_at
            or request.observed_at - decision.occurred_at > timedelta(hours=6)
            for decision in request.recent_decisions
        )
    ):
        raise HTTPException(status_code=422, detail="timestamps must include a timezone")
    try:
        if Decimal(request.max_proposal_notional) <= 0:
            raise HTTPException(status_code=422, detail="max_proposal_notional must be positive")
    except InvalidOperation:
        raise HTTPException(
            status_code=422, detail="max_proposal_notional must be a decimal"
        ) from None
    context = request.model_dump(
        exclude={"provider", "credential", "profile", "safety_identifier"}, mode="json"
    )
    context["objective"] = request.objective.strip()
    context["allowed_symbols"] = allowed
    try:
        result = await registry.get(request.provider).propose_shadow(
            request.credential, request.profile, context, request.safety_identifier
        )
        return {
            "decision": {
                "decision": result.decision,
                "symbol": result.symbol,
                "side": result.side,
                "proposed_notional": result.proposed_notional,
                "confidence": result.confidence,
                "thesis": result.thesis,
                "risk_flags": list(result.risk_flags),
                "limitations": list(result.limitations),
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
