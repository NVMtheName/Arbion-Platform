# Offline order lifecycle and cash-matching laboratory

## What is implemented

The Go `internal/executionsim` package is an executable, restartable simulation of order-state and transaction-matching mechanics. This is functional domain code with a durable local event journal, not another readiness report. It accepts only explicitly simulation-labelled fixtures and is called only by tests and `cmd/simulate-lifecycle`.

It has **no financial-provider client, AI call, credential input, production database, HTTP route, scheduler hook, or real-order submission method**. The command accepts no account or trade arguments. It creates its own private temporary directory and retains the fictional journals for inspection. It is not built into the production runtime image.

Coinbase and Schwab scenarios use the same provider-independent engine with different synthetic account/provider/symbol identities. Prices, quantities, and transfers are fictional. In particular, the fixture SPY price and fractional fills are not claims about Schwab-supported products, sizing, order behavior, or current market data. These are normalized laboratory fixtures, **not actual Coinbase/Schwab API adapters**.

## Lifecycle behavior

| Saved event                                                        | Result and reservation behavior                                                                     |
| ------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------- |
| Order opened                                                       | REGISTERED; reserve exact maximum buy cash including fees or exact sell quantity.                   |
| Send recorded                                                      | OUTCOME_UNKNOWN immediately. Submission is not acknowledgment or fill. No resend transition exists. |
| Matching acknowledgment                                            | ACKNOWLEDGED; bind one unique synthetic order identity. No ledger settlement.                       |
| Partial fill settled                                               | Apply exact incremental quantity, price, and fee once. Retain the remaining reservation.            |
| Cancel requested                                                   | CANCEL_PENDING; retain the reservation. A matching fill may still arrive.                           |
| Remaining quantity filled                                          | FILLED, even if cancellation was pending. Release unused reservation.                               |
| Cancel confirmed with matching terminal totals                     | CANCELLED; keep earlier fills and release only the remaining reservation.                           |
| Rejection of an unknown attempt with explicit zero terminal totals | REJECTED; release reservation without inventing a fill.                                             |
| Missing or mismatched terminal totals                              | Reject the input; keep the pending/unknown state, version, cash and quantity claims unchanged.      |

Every new order event requires the exact next per-order version, a nondecreasing order timestamp, and the same owner/account/provider/run identity. Out-of-order or missing revisions, unknown orders, conflicting provider-order identities, overfills, and impossible transitions fail closed without changing the projection. This laboratory does not buffer or reorder provider messages and does not implement replacements, trade busts, fees posted later, settlement delay, fractional-product rules, options, margin, shorting, or live retries.

### Terminal settlement fence

A cancellation status alone is insufficient to release reserved buy cash or sell units. `CANCEL_CONFIRMED` now requires an explicit `terminal_settlement` witness containing cumulative `filled_quantity`, `gross_notional`, and `fees`. The engine independently accumulates those values from each uniquely applied incremental fill using exact decimal arithmetic. All three terminal totals must match; an unknown attempt's `REJECTED` event requires explicit zero in all three fields. Omitted fields never mean zero. Numerically equivalent trailing-zero spellings compare exactly, while changed facts under a previously accepted delivery identity still conflict.

For example, if cancellation reports 1.5 filled units but the journal contains only 1 unit, the engine returns `ErrSettlement` and preserves the outstanding claim. It does not manufacture the missing half-unit fill, apply terminal totals as a second settlement, consume the next revision, or resend an order. The harness must first provide the missing independently identified fill and then present the terminal witness at the correct next revision. The same behavior survives reopening the journal. A write failure cannot publish a reservation release, and concurrent duplicate confirmations have only one effect.

These are **fictional normalized terminal facts**, not a Coinbase or Schwab API schema or proof of complete provider history. They assume each synthetic fill's fee is final and known in the fixture cash currency. Actual terminal status, fill completeness, fee currency/finality, and any later correction still require a documented provider adapter and reconciliation design. Invalid attempted terminal input is not appended as an accepted fact; a future real event-ingestion layer must preserve unresolved inbound observations separately. No existing real observation is used to invent these totals.

## Economic matching and exact money

Delivery IDs identify received facts. A separate stable transaction ID identifies a single incremental fill or settled deposit/withdrawal. Repeated delivery is a no-op. The same economic event under a new delivery ID is journalled as an alias without settling it again. Reusing either identity with changed facts fails closed, including after restart. Decimal spellings are intentionally strict: a differently spelled payload requires normalization before it can match; no provider-specific normalization is inferred here.

BUY settlement subtracts exact `quantity × price + fee`; SELL settlement adds exact `quantity × price − fee`. Quantity and cash use rational arithmetic, never binary floating point. Inputs permit at most 10 decimal places, and an unrepresentable product is rejected rather than silently rounded. This deliberately does not replace the existing production Paper adapter's explicit rounding policy.

The fixture configuration fixes a per-order ceiling, minimum cash reserve, allowed symbols, and cumulative buy-spend ceiling. Deposits and sale proceeds do not raise those ceilings. Multiple open buys cannot claim the same cash and multiple open sells cannot claim the same units. A settled withdrawal can consume reserve headroom: the withdrawal remains recorded, pending claims stay reserved, and the snapshot exposes `funding_review_required`, blocking new opens and sends. A withdrawal larger than total cash, or a fill that would make cash negative, is inconsistent with this cash-only fixture and fails closed; that error is **not** an assertion that a real external transaction did not occur. No cause is inferred from balance changes.

These fixture limits are not production authorization, a RiskEvaluation, or a mandate. They must never substitute for the Go authorization/risk/capital gate. The existing engines' limits are untouched.

## Durable restart and failure behavior

The private JSONL journal contains a genesis configuration and hash-linked canonical records. A nonblocking OS file lock excludes other processes and a mutex serializes callers on one handle. Each valid new delivery is written and fsynced before its state is published or success acknowledged. The initial directory entry is also synced. Opening replays the complete chain under the exact original scope/configuration; no mutable checkpoint can bypass replay. Maximum size is 16 MiB and the limit is 10,000 distinct delivery IDs.

A process exiting after a durable attempt but before receiving its result reopens in OUTCOME_UNKNOWN with its reservation intact. A response-lost redelivery cannot create another attempt. After a storage error the handle stops serving or accepting state until reopened. Torn trailing bytes, changed records, duplicate sequence/identity, missing middle records, future timestamps, unknown JSON fields, noncanonical JSON, mismatched configuration, or invalid replayed transitions fail closed. Original evidence is never automatically truncated or repaired. Invalid attempted inputs are returned as errors and are not appended as accepted financial facts.

The hash chain detects accidental inconsistency, **not malicious rewriting, complete-tail truncation, or rollback by a filesystem owner**. This is not authenticated audit archival, a production database transaction design, a high-availability journal, or a substitute for PostgreSQL. The implementation targets the project's macOS development and Linux CI environments.

Older fictional journals containing cancellation/rejection records without the terminal witness now fail replay closed and remain untouched. There is no inferred backfill or automatic rewrite. Journals containing only otherwise valid nonterminal events can still replay and continue; new fixture scenarios use a fresh private journal. Production Paper records and provider observation tables are not read or migrated by this change.

## Run now, without scheduled cycles

From `services/api`, with the Go version pinned by `go.mod`:

```sh
go test -race ./internal/executionsim
go run ./cmd/simulate-lifecycle
```

Both synthetic scenarios test an uncertain attempt and restart, acknowledgment, partial fill and restart, duplicate fill delivery, premature cancellation totals rejected before a missing fill (including restart with the reservation intact), cancellation with an intervening fill, matched confirmed cancellation, duplicate deposit, withdrawal, partial/full sale, explicit zero-settlement rejection, and final full replay. Each ends with exactly 17 applied lifecycle/economic events, six unique settled transactions, three terminal orders, zero remaining reservations, and USD 1,077.3850000000 fictional cash. This balance includes a fictional deposit/withdrawal and artificial prices; it is not strategy performance. The journal additionally preserves two duplicate delivery aliases and genesis. Tests separately cover process exit without Close, competing writers, concurrent duplicate terminal delivery, failed terminal writes, legacy terminal evidence, changed/missing quantities and fees, cross-account terminal identity, sell-quantity reservation, corruption, funding shortfalls, double reservation, overselling, and exact decimal rejection. CI runs the complete tests and executable scenarios without network calls from the harness.

## Next integration work

The first production evidence step is now implemented separately: [Private Fill Evidence](PRIVATE_FILL_EVIDENCE.md) captures normalized Coinbase observations during an existing authorized history read, with account-bound identity, duplicate matching, and immutable database safeguards. It does not feed this laboratory, prove complete history/settlement units, or implement a Schwab wire adapter. The following gates remain:

1. Map **read-only, documented provider transactions and order history** into separately tested normalized evidence, with explicit stable IDs, corrections, pagination completeness, settlement/fee semantics, and supported product precision. Do not guess transaction cause from snapshots.
2. Bind a durable PostgreSQL simulation lifecycle to existing Go risk/mandate/capital evidence and test concurrency/crash boundaries before replacing the current immediate-fill Paper path. Keep existing history intact.
3. Prove that external-account facts can be ingested without changing strategy allocation, duplicating fills, or requesting routine owner approval for understood cash movements. Keep genuinely unmatched or inconsistent evidence reviewable.

Actual broker submission, cancellation, automated execution approval, and live-enabling work remain separate architecture/security-reviewed tasks. This laboratory changes none of those boundaries.
