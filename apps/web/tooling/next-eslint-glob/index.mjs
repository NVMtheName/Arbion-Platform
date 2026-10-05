// The pinned Next ESLint plugin imports fast-glob only for optional
// settings.next.rootDir matching. Arbion runs ESLint in apps/web and uses
// context.cwd, so every existing rule runs without that optional dependency.
// This is intentionally NOT a glob implementation: custom root configuration
// must fail loudly, never silently omit projects or weaken a lint rule.
// Remove this version-scoped guard after an upstream safe migration; do not
// add rootDir patterns or broaden its API without a reviewed replacement.
export function globSync() {
  throw new Error(
    "Custom Next ESLint rootDir globbing is unavailable. Run lint from apps/web without settings.next.rootDir; custom roots require a reviewed safe migration.",
  );
}
