# E*TRADE connector foundation

## Delivery boundary

`services/api/internal/financial/etrade` implements a tested, **unwired** authorization and account-discovery foundation. E*TRADE remains `PLANNED` in the provider registry. There are no new application routes, browser credential fields, deployment secrets, database migrations, scheduler changes, or production provider calls. Existing Coinbase and Schwab behavior is unchanged.

This is not a complete `financial.BrokerProvider`: balances, positions, secure durable credentials, owner-scoped lifecycle wiring, and the website connection flow remain to be implemented. No production connection or provider certification is claimed from mock tests. It has no preview, order, transfer, cancellation, market refresh, AI cycle, or live-execution surface.

## Implemented behavior

- Explicit sandbox or production configuration; no implicit production default or arbitrary endpoint URL. Sandbox observations carry `environment: sandbox` and `synthetic: true`, even when the account set is empty. They must never enter real-account holdings or AI evidence.
- OAuth 1.0a GET signing with RFC 3986 escaping, encoded-key/value sorting, fresh cryptographic nonces, timestamp, and the provider-required HMAC-SHA1 signature. OAuth parameters stay in the Authorization header. The signature is checked against E*TRADE's published known-answer fixture.
- A five-minute pending authorization, distinct from an access authorization, bound to the exact consumer credentials and environment. Exchange consumes it once, including failed exchanges; concurrent calls cannot reuse it. The consent URL includes only the provider-required consumer key and temporary token, never either secret or an access token. Do not log this URL.
- Access-token expiry at the next Eastern calendar midnight, with DST-aware calendar arithmetic and embedded timezone data. Same-day renewal does not extend that expiry. Expired or future-dated authorizations fail locally. There is no hidden retry or automatic renewal loop.
- Bounded HTTP responses and timeouts, no cookies or redirects, no raw provider error bodies or transport errors retained. Credential objects are opaque and excluded from ordinary JSON output and pointer formatting.
- Complete account discovery or a classified failure, never partial success. Missing/null response containers, duplicate account identities, ambiguous JSON members, and malformed status/type fields fail closed. HTTP 204 and an explicit empty account array are distinct supported empty results. Full account numbers and opaque provider account keys never enter the JSON response; labels use masked identifiers. Account discovery does not invent base currency, holdings freshness, buying power, margin permission, or trading authority.

## Provider facts and references

Reviewed against the official documentation on 2026-09-11:

- [Developer guide](https://developer.etrade.com/getting-started/developer-guides): OAuth 1.0a, signature fixture, sandbox canned data, and the shared authorization server. Sandbox keys and access authorizations are separate from production; data endpoints use `apisb.etrade.com` versus `api.etrade.com`.
- [Request token](https://apisb.etrade.com/docs/api/authorization/request_token.html): GET `/oauth/request_token`, five-minute lifetime, and `oauth_callback=oob` even for preconfigured callbacks.
- [Application authorization](https://apisb.etrade.com/docs/api/authorization/authorize.html): browser consent at `https://us.etrade.com/e/t/etws/authorize`, using `key` and `token`; the verifier may be manually returned or delivered through a provider-registered callback.
- [Access token](https://apisb.etrade.com/docs/api/authorization/get_access_token.html) and [renewal](https://apisb.etrade.com/docs/api/authorization/renew_access_token.html): inactivity after two hours is different from default midnight-Eastern expiry. Renewal reactivates the same token; daily expiry needs a new sign-in. The malformed sandbox authorization URLs printed on individual endpoint pages are not copied; the guide explicitly specifies the shared authorization server.
- [Account discovery](https://apisb.etrade.com/docs/api/account/api-account-v1.html): account-list endpoint, account identities/types/statuses, and the explicit 204 no-records result.

## Next integration milestones

1. Add exact-decimal balances and fully paginated positions with provider/account identity validation, explicit completeness, currency, asset-type and timestamp provenance. Do not claim broker-reported price or performance fields that are absent.
2. Design the financial Vault payload and short-lived pending-flow storage. Bind each start and completion to the initiating authenticated owner and entitlement; atomically consume server-side flow state before exchange. An in-memory mutex is not a distributed callback replay defense. Never deserialize or trust a caller-supplied authorization object.
3. Wire authorization, renewal, discovery, resync, and disconnect through the existing owner-scoped lifecycle locks. Verify account continuity and cross-provider isolation, preserve inventory on transient errors, and distinguish renewal from daily reauthorization. Website login/logout must not mutate provider authorization.
4. Add an Arbion-branded connection flow with manual verifier fallback unless the exact callback is confirmed with E*TRADE. Make sandbox versus real accounts unmistakable. Only advertise availability when this complete path passes integration and security checks.

Before any real-account proof, use securely supplied environment-appropriate credentials and explicit user consent. No credentials are needed for these offline tests. Supporting unrelated Arbion customers through an app-owned vendor key also depends on E*TRADE's vendor approval process; this foundation does not grant that approval or any trading authority.

## Verification

Run from `services/api`:

```sh
go test -race ./internal/financial/etrade
go vet ./...
go test -race ./...
```

Tests substitute the HTTP transport and never contact E*TRADE. They cover the official signature fixture, escaping and parameter ordering, both environments, request/access separation, one-use exchange races, expiry across DST, denied renewal after midnight, account masking and completeness, response bounds, redacted errors, redirects/cookies, and a conservative exported method/registry surface.
