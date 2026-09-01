import { cookies } from "next/headers";
import Link from "next/link";
import { redirect } from "next/navigation";

import { AppPageHeader } from "../app-page-header";
import { JournalList, type JournalEntry } from "./journal-list";

type JournalResponse = {
  entries: JournalEntry[];
  next_cursor: string;
  live_execution_available: false;
};

export default async function ActivityPage({
  searchParams,
}: {
  searchParams: Promise<{ cursor?: string }>;
}) {
  const { cursor = "" } = await searchParams;
  const jar = await cookies();
  const api = process.env.API_BASE_URL ?? "http://localhost:8080";
  const query = new URLSearchParams({ limit: "25" });
  if (cursor) query.set("cursor", cursor);
  const response = await fetch(`${api}/api/decision-journal?${query}`, {
    headers: { cookie: jar.toString() },
    cache: "no-store",
  });
  if (response.status === 401) redirect("/login");
  if (!response.ok) {
    return (
      <main className="journal-page command-content-continuity">
        <AppPageHeader />
        <p className="eyebrow">DECISION JOURNAL</p>
        <h1>Journal unavailable</h1>
        <p className="lede">
          Arbion could not load this page of activity. Return to the newest
          entries and try again.
        </p>
        <Link className="button-link" href="/activity">
          Newest entries
        </Link>
      </main>
    );
  }
  const data = (await response.json()) as JournalResponse;

  return (
    <main className="journal-page command-content-continuity">
      <AppPageHeader />
      <p className="eyebrow">DECISION JOURNAL</p>
      <h1>Every decision, in context.</h1>
      <p className="lede">
        Review what each strategy proposed, how the deterministic risk gate
        responded, and what the non-live adapter recorded.
      </p>
      <p className="journal-safety">
        <strong>READ-ONLY · LIVE EXECUTION UNAVAILABLE</strong>
        PAPER entries are simulations. SHADOW entries show what Arbion would
        have submitted. Neither mode sends a broker order.
      </p>

      <JournalList entries={data.entries ?? []} />

      <nav className="journal-pagination" aria-label="Decision journal pages">
        {cursor ? <Link href="/activity">← Newest</Link> : <span />}
        {data.next_cursor ? (
          <Link
            href={`/activity?cursor=${encodeURIComponent(data.next_cursor)}`}
          >
            Older entries →
          </Link>
        ) : (
          <span>End of journal</span>
        )}
      </nav>
    </main>
  );
}
