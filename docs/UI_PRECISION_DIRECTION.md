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
content changes are scoped to the landing and dashboard. Do not migrate every
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

Visual QA uses a temporary, explicitly labeled local fixture because the production
session is signed out. Test desktop, 390px and 320px widths, doubled root text size,
partial and unavailable states, filters, and the optimized build. Remove all fixture
routes and generated development-only instructions before the final production
build and commit. Do not publish fixtures or bypass authentication.

Apply the same density and hierarchy next to Connections and AI Operations;
verify each with real existing component fixtures before release. Richer market
charts should use actual source-attributed data, not decorative growth curves.
Reserve any future WebGL scene for an optional lazy-loaded public marketing
accent with reduced-motion and non-WebGL fallbacks; do not add it to the
financial working surface merely to create motion.
