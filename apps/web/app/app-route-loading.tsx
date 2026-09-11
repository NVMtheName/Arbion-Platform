import { AppPageHeader } from "./app-page-header";

// Prefetch only the public shell. Account data still loads through each page's
// existing authenticated, no-store request path; the fallback makes no requests.
export function AppRouteLoading({ section }: { section: string }) {
  return (
    <main className="app-route-loading command-content-continuity">
      <AppPageHeader
        contentHeadingId="route-loading-title"
        showConnectionHealth={false}
      />
      <section
        className="app-route-loading-content"
        aria-busy="true"
        aria-labelledby="route-loading-title"
      >
        <p className="eyebrow">YOUR WORKSPACE</p>
        <h1 id="route-loading-title">{section}</h1>
        <p role="status">Loading your workspace…</p>
        <div className="app-route-skeleton" aria-hidden="true">
          <div />
          <div />
          <div />
        </div>
      </section>
    </main>
  );
}
