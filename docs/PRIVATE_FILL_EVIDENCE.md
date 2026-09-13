# Private provider fill observations

## Implemented boundary

The existing owner-authorized Coinbase spot trade-history read now retains a private projection of the same response. It makes no additional provider requests. After the normal account/entitlement/vault checks, Go attempts a one-second-bounded database capture, strips the private projection, and returns the existing public history plus `evidence_capture_status`: `SAVED`, `UNAVAILABLE`, or `CONFLICT`. A failed capture does not disable the connection, hide otherwise valid display history, change holdings, or alter reconciliation, capital, mandate, risk, or scheduler state. An explicitly foreign portfolio in the provider response fails the whole read closed.

This is **an observation store, not settlement accounting or execution authority**. No observation is an Arbion order intent, simulated fill, confirmed cash movement, strategy-created trade, approval, or live order. The offline lifecycle laboratory and production Paper/Shadow paths remain separate.

## Source and exactness

The adapter follows the [Coinbase Advanced Trade List Fills reference](https://docs.cdp.coinbase.com/api-reference/advanced-trade-api/rest-api/orders/list-fills), reviewed September 13, 2026. It preserves documented entry/trade/order identity, permissioned retail portfolio, product, side, price, size and its explicit `size_in_quote` unit, commission amount, liquidity, trade time, and sequence time. Tests use synthetic documented-shape responses, not captured credentials or real account payloads.

The endpoint has no explicit commission-currency field. Private evidence therefore stores the reported decimal commission amount with `commission_currency_status=UNAVAILABLE`; it does **not** adopt the existing public display's quote-currency convention as exact evidence. Fee-adjusted settlement and net cash cannot be derived from this contract. Quote-sized fills remain quote-sized; no base quantity is reconstructed.

Decimal input permits up to 40 integer and 32 fractional digits without exponents, negatives, binary floating point, or rounding. Equivalent trailing-zero spellings normalize to the same digest. PostgreSQL uses unconstrained exact numeric plus explicit scale/range checks so invalid precision is rejected rather than rounded before validation. Trade and sequence timestamps retain RFC3339 nanoseconds as text. Future, missing, malformed, or contradictory evidence is rejected. Capture/read timestamps are separate observation times, not execution facts.

## Private identity and durable capture

Raw provider account, entry, trade, and order identifiers never enter the observation tables, browser projection, AI request, or audit metadata through this path. Versioned SHA-256 references are bound to provider, provider-account identity, and identifier kind. These hashes support equality within the account; they are not encryption, anonymization, proof of authenticity, or authority. Existing encrypted connection/account identity storage is unchanged.

`financial_fill_observations` is owner/account scoped and append-only. A unique account/entry reference prevents duplicate insertion. The digest covers normalized identifiers, decimal facts, units, explicit fee-currency unavailability, and exact trade/sequence times, but excludes the later retrieval time. A subsequent equal entry matches the saved row; changed facts under the same entry conflict and roll back the entire incoming batch. Changed provider facts must later receive an explicit correction workflow, never overwrite history. Different entry IDs are distinct observations even when a trade ID repeats; this milestone does not infer aliases or settle them economically.

`financial_fill_capture_receipts` records each successful bounded read's reported, unique, new, and matched counts plus `has_more`. PostgreSQL rejects updates/deletes, enforces owner/account/provider binding, and refuses to drop nonempty evidence through the migration rollback. Concurrent duplicates insert one observation. Missing identity or storage failure produces only a safe status; failure audit metadata contains the internal account ID and fixed category, never a raw database/provider error. No raw error is returned to the owner.

The normal history page is at most 50 rows and is not complete account history. Every receipt permanently records `complete_account_history=false`, including empty or terminal pages. No receipt links a deposit, withdrawal, position change, or strategy action to an observed fill.

## Bounded private pagination primitive

Coinbase also implements a server-only `FillEvidenceProvider.ReadFillEvidence` interface for a fixed window of at most 24 hours, a 30-second context budget, and at most ten 100-row pages. It re-verifies the key's permissioned portfolio, requests only SPOT fills, omits optional unstable sorting, validates every returned portfolio and trade-time window, detects repeated cursors, deduplicates identical entries, and discards the complete partial result on an error or conflict. Cap status, page count, fixed window, and observed cursor exhaustion stay explicit.

**Cursor exhaustion is not proof of complete account history or provider snapshot isolation.** The default ordering and a fixed window do not establish either guarantee. No cursor, raw response, or private reference reaches the browser. This pagination primitive is tested but is not routed, scheduled, or used by the 50-row capture path in this release.

## Remaining integration gates

1. Validate real, owner-authorized capture evidence as it naturally arrives; fixture success does not prove production coverage.
2. Define correction/alias handling, complete coverage, exact fee/settlement units, order-state matching, and separately documented deposit/withdrawal facts before deriving cash or position changes.
3. Add Schwab's actual read-only transaction adapter only from an accessible authoritative schema. Current Schwab-labelled lifecycle fixtures are not wire-compatibility proof; this release adds no Schwab fill parser.
4. Bind complete evidence into the restartable simulation lifecycle and deterministic risk/capital controls before any future execution work. Submission/cancellation/live activation still require the separate architecture and security gates in [Execution Engine](EXECUTION_ENGINE.md).

## Verification

Unit tests cover exact decimal aliases and conflicts, private serialization, quote-sized fills, unavailable fee currency, owner-service stripping, failure isolation, foreign portfolio rejection, pagination duplicates/loops/caps, missing identity/units, malformed data, and late provider errors. The isolated PostgreSQL CI test covers complete migration application, append-only enforcement, owner/account binding, decimal/timestamp preservation, atomic conflict rollback, concurrent duplicate reads, and zero authority/strategy creation. No test or release step triggers a real provider history read, AI cycle, broker order, or holding change.
