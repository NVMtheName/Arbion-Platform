# Arbion precision interface

## Direction

Charcoal instrument surfaces, Arbion cyan, clear editorial typography, restrained
lighting and fixed-size tactile navigation. Portfolio values and current AI
conclusions take priority over diagnostics. Use existing Arbion SVG branding.

The September 2026 reference review covered [ThreeUI Community](https://github.com/MengTo/threeui),
especially its Animated Top Dock and Diagnostics Panel. The dock's dimensional
surface treatment is relevant; its cursor-driven width/height growth is not:
Arbion's navigation targets must stay in place. The full package also carries
multiple Three.js runtime versions and several document-based renderers. This
pass adds no ThreeUI dependency, copied renderer, remote asset, iframe, shader,
font service, or third-party network request. The implementation is native
React/CSS using the existing Motion configuration and reduced-motion policy.

## First delivered surface

- Shared navigation retains route-aware state, scroll retention, touch access,
  skip link, connection-health indicator and stable header dimensions.
- Landing explains accounts → model → strategy, with an accessible three-view
  product illustration. Sample portfolio values are explicitly fictional.
  Tabs support arrows, Home and End, maintain one tab stop and reserve a common
  height. No chart pretends to show live prices or investment returns.
- Dashboard moves the real connected portfolio before the detailed AI section.
  Compact saved operating/input summaries expand automatically when attention,
  unavailable or inconsistent evidence is present. Full existing evidence and
  links remain available through native disclosures.
- AI cards prioritize the latest saved conclusion and exact recorded route.
  Schedule, telemetry and outcome evidence remain in a disclosure that opens
  for missing evidence or review conditions. Paper and Shadow stay distinct.
- Existing API requests, auth, credential handling, exact money calculations,
  scheduling, risk checks and execution boundaries are unchanged.

## Styling boundary

`app/precision.css` is imported after the legacy stylesheet at the root so the
cascade is identical on every route and in the production build. Global changes
are restricted to base surface tokens, page background and the shared header;
content changes are scoped to the landing, dashboard, Portfolio, Connections and
AI Operations. Do not migrate every
legacy panel in a single unverified rewrite. Later route work should consolidate
styles into the same surface vocabulary and remove superseded rules as it goes.

## Verification

Test the actual production build in addition to development rendering. Check
desktop and mobile, keyboard tabs, fixed preview height, disclosure visibility,
unavailable/partial values and no horizontal page overflow. Preserve the stable
1440px app shell and responsive header across routes. Private dashboard visual
fixtures must never be committed, deployed or represented as account evidence.

## Next visual work

### Portfolio continuation

The Portfolio workspace now uses a compact page heading, a primary observed-value
surface, readable summary coverage, tactile provider filters, explicit result
counts and a clear-filter action. Summaries always describe all loaded positions,
not the current filter. The exact existing monetary helpers and account-scoped
GET requests are unchanged. Missing account-list responses are distinct from
successful empty lists; individual unavailable accounts remain visibly flagged.
The shared header and skip target remain intact after extracting the testable
workspace from the server-loading page.

The desktop ledger keeps the asset column anchored during horizontal scrolling.
Phone cards retain every field, provider/source description and unavailable value.
Account links have specific accessible names. Filter buttons expose pressed state,
results announce changes, and the table retains explicit column headers/caption.
No false live-price indicator, additional provider request or refresh action is added.

Visual QA uses temporary, explicitly labeled local fixtures for edge cases. The
released Portfolio was also verified through the existing signed-in session.
Test desktop, 390px and 320px widths, doubled root text size,
partial and unavailable states, filters, and the optimized build. Remove all fixture
routes and generated development-only instructions before the final production
build and commit. Do not publish fixtures or bypass authentication.

### Connections continuation

Connections now opens on a compact heading and a tactile four-step setup strip.
Financial provider forms, AI provider setup and default-model choice precede the
long saved operating evidence. Every existing diagnostic view and automatic
attention disclosure remains intact, with a direct health link and visible
authorization/inventory attention banner. Empty health guidance makes the review
anchor useful before the first connection. Provider cards use charcoal surfaces,
readable fields, 44px controls and specific accessible provider/action names.
Section links clear the sticky navigation at mobile and enlarged text sizes.

The testable workspace is extracted from the existing server-loading page.
Existing fetches, cookie handling, credential inputs, key validation, provider
authorization, model-loading, mutations and runtime protection remain unchanged.
When the saved financial inventory is unavailable, the page does not offer an
empty-state connection form or claim an existing connection is healthy. A missing
provider catalog or default-model preference is explicitly unavailable. Planned
providers stay unavailable; completing setup is not trading authorization.

### AI Operations continuation

Automations now opens with a compact AI Operations heading and an explicit
setup link, followed by readable fleet counts and the existing engine deck.
Dimensional charcoal cards prioritize exact account/provider identity, a visible
Paper/Shadow label, the existing health signal, newest saved decision and next
guarded cycle. Engine titles are semantic headings; account names and model
routes wrap instead of being truncated. Next-cycle, scheduler, model, universe
and exact capital facts retain their existing values and unavailable behavior.

Each engine's specifically named immutable-evidence link comes before its long
diagnostics. Quote, Paper gate, exposure/outcome and advanced runtime evidence
are unchanged; native disclosures retain automatic review opening. No projection,
data-loading, risk, provider, scheduling or execution handler changed. Setup links
do not run an engine. Missing current context is no longer described as “live.”

Verify desktop, narrow screens, 200% text, unavailable inventory, saved attention
and empty states using labeled existing-component fixtures; remove fixtures before
the final build and release. Apply this same focused hierarchy next to Activity's
saved decision journal, retaining exact provenance and review controls. Richer market
charts should use actual source-attributed data, not decorative growth curves.
Reserve any future WebGL scene for an optional lazy-loaded public marketing
accent with reduced-motion and non-WebGL fallbacks; do not add it to the
financial working surface merely to create motion.

### Shared palette and route continuity

All legacy neutral foundations now reference the same charcoal surface, slate
border, muted text and high-contrast foreground tokens used by the precision
pages. This is a color-only migration of the legacy stylesheet: selectors,
layout declarations and behavior are unchanged. Saturated warning, danger,
gain/loss, Paper/Shadow and provider identity colors remain meaningful; primary
buttons and navigation accents use cyan. New surface colors must use these
tokens instead of adding a page-specific green or blue neutral palette.

The shared header no longer defaults to a redundant Dashboard back link. The
Dashboard tab, brand link, explicit contextual returns and custom account
actions remain. The action slot retains its dimensions even when empty.

Every primary signed-in destination (including the legacy Connections alias and
Risk settings) now has a lightweight loading boundary. Next can prefetch the
data-free shell and stream content without waiting for all page requests. It
contains navigation and an honest loading status, not cached account values or
placeholder balances; it does not request connection health. Existing page
authentication, no-store requests, owner scoping and financial freshness rules
are unchanged. Navigation remains interruptible. This improves response feedback,
not the speed of the underlying provider or model.

Working pages use a 160ms opacity-only arrival, excluding the shared header and
skip target, and disable it for reduced motion. Dashboard, market and crypto
account entry panels no longer start hidden or wait through staggered reveals.
Non-landing Motion defaults are 180ms; the public storytelling pace is unchanged.
Verify slow streaming and final content retain identical header geometry, along
with desktop, narrow mobile, enlarged text and existing semantic warning states.

### Activity continuation

The Decision Journal now opens as a saved-record workspace with a compact heading,
one page-scoped count strip, readable wrapping filters and explicit visible-result
context. Saved decisions precede the longer AI comparison index; a direct anchor
reaches the unchanged comparison evidence without hiding its attention states.
Charcoal record surfaces use the shared palette, while Paper/Shadow labels and the
existing review predicate retain their distinct meaning. Review-required records
stay open automatically and receive an amber edge. Exact record links, copy actions,
account identity, rationale, risk, execution and mandate evidence remain intact.

The server's owner-scoped no-store request, authentication redirect, exact-record
identity checks and cursor/filter return URLs are unchanged. Regression tests cover
missing, substituted and ambiguous exact evidence separately from successful empty
pages. Empty guidance explains the saved activity path without inviting a manual
cycle. No provider refresh, model call, financial calculation or mutation is added.

Visual verification uses labeled local fixtures for saved, empty, filtered-empty,
focused, unavailable, long-label and missing-provenance states. Verify desktop,
narrow 390px/320px frames, doubled root text and direct comparison-link clearance.
Temporary fixture routes must be removed before the final production build.
An expired production session is a protected-page visual QA limitation, not a reason
to bypass authentication or replace saved owner evidence with illustrative data.

### Capital continuation

Capital now begins with a compact budget-workspace heading, readable account and
reservation counts, and a direct in-page jump to the existing creation form.
Account surfaces use the shared charcoal palette. Five exact account-policy facts
form a consistent strip; each named budget has readable full-precision values and
label/value rows instead of narrow, cramped columns. Paper simulation and Shadow
claim badges are visibly distinct, with protected reserves retaining their warning
edge. An unknown provider never receives a decorative Schwab initial.

Named account regions and budget articles support accessible review. The existing
currency-mismatch and invalid-decimal predicates now also flag account cards and
announce their unchanged warnings. Archive disclosure remains keyboard accessible.
Form labels, hints and controls are larger and align consistently; the submission
handler, validation, account eligibility and state transitions are unchanged.
The four owner-scoped no-store inventory requests and all exact arithmetic,
currency separation, claim bases and unavailable behavior remain intact. The jump
link neither submits the form nor authorizes trading.

Verify desktop, 390px/320px responsive documents, enlarged text, long names,
full-precision amounts, protected reserves, mixed currency, invalid decimals,
unavailable/empty/disconnected inventories and keyboard archive access. Check
header coordinates across both long and short states. Use only labeled local
fixtures when production authentication has expired; remove them before release.

### Security continuation

Security now uses a compact reusable “Security & access” introduction and direct
in-page links to authenticator, password, browser-session and saved-activity
sections. The desktop MFA card spans the two adjacent control rows so its height
does not force the sessions card below the entire MFA block. Narrow screens use
the unchanged document order in one column. Labels, controls, session timestamps
and activity categories are readable; the session-count surface uses neutral
charcoal rather than green. Dangerous buttons retain explicit labels and a
distinct warm warning treatment. Unavailable sessions and activity remain visible
with review edges and their existing explanations; no panel is collapsed.

The four owner-scoped no-store requests, authentication redirects, required-data
failure behavior, session validation, UTC timestamps, activity mapping/cursors,
MFA/password/session handlers, form requirements, busy/disabled states and recovery
code lifecycle remain unchanged. The extracted introduction has no data requests
or client state. This visual work does not strengthen security controls or imply
compliance. Inert, clearly labeled local fixtures are for visual review only;
never submit actual security actions, enter real credentials, or generate recovery
codes for UI QA. Mocked unit tests verify request boundaries and transient flows.
Remove fixtures before final build/commit, verify header geometry and anchor
clearance at narrow/enlarged-text sizes, and record any signed-in QA limitation.

### Markets continuation

Markets now uses a compact introduction, direct links to the watchlist, equity
and options desk, crypto board, filings and source verification, and readable
charcoal surfaces matching the other working pages. Summary facts stay together
in a compact strip; watchlist asset names and monetary values wrap rather than
truncate. All research controls have readable labels and 44px targets. Exact
provider, feed, quality, venue and time evidence remains visible, with existing
unavailable, partial, degraded and aged states retaining distinct treatment.
The option-chain table remains independently scrollable with its existing hint.

The workspace is extracted from the server loader without changing requests,
validation, fallback or authorization behavior. Existing watchlist mutations,
account selection, quote/option/filing handlers, crypto polling, source/history
refresh intervals and numerical helpers are unchanged. Timer tests verify the
existing five-second, thirty-second and five-minute cadences and cleanup.
Source metadata, account inventory and market observations remain separate.
No new request, dependency, chart data, provider capability or trading path is
introduced. Existing loader fallback limitations are a separate follow-up, not
silently rewritten by a visual pass.

For visual QA, mount actual components only behind local mocks installed before
rendering them: the research board otherwise requests crypto observations on
mount. Block all fixture API traffic from reaching real endpoints and reject
mutations. Clearly label illustrative values, remove every fixture before the
final build/commit, and test desktop, narrow responsive documents, enlarged text,
long labels, unavailable/partial/empty states, keyboard anchors and stable header
geometry. Do not submit real research, refresh, account or trading actions for UI
verification. Preserve authentication if the production session is expired.
