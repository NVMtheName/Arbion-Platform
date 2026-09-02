import { cookies } from "next/headers";
import Link from "next/link";
import { redirect } from "next/navigation";

import { AppPageHeader } from "../app-page-header";
import {
  activityJournalHref,
  JournalList,
  normalizeJournalFilter,
  type JournalEntry,
} from "./journal-list";

type JournalResponse = {
  entries: JournalEntry[];
  next_cursor: string;
  live_execution_available: false;
};

function JournalUnavailable({
  exact,
  returnHref,
}: {
  exact: boolean;
  returnHref: string;
}) {
  return (
    <main className="journal-page command-content-continuity">
      <AppPageHeader contentHeadingId="activity-page-title" />
      <p className="eyebrow">DECISION JOURNAL</p>
      <h1 id="activity-page-title">
        {exact ? "Exact record unavailable" : "Journal unavailable"}
      </h1>
      <p className="lede">
        {exact
          ? "This owner-scoped record link is invalid, unavailable, or outside this account. Arbion does not reveal or substitute another record."
          : "Arbion could not load this page of activity. Return to the newest entries and try again."}
      </p>
      <Link className="button-link" href={returnHref}>
        {exact ? "Return to journal context" : "Newest entries"}
      </Link>
    </main>
  );
}

export default async function ActivityPage({
  searchParams,
}: {
  searchParams: Promise<{
    cursor?: string;
    decision?: string;
    view?: string;
  }>;
}) {
  const { cursor = "", decision = "", view } = await searchParams;
  const filter = normalizeJournalFilter(view);
  const returnHref = activityJournalHref({ cursor, filter });
  const jar = await cookies();
  const api = process.env.API_BASE_URL ?? "http://localhost:8080";
  const query = new URLSearchParams();
  if (decision) {
    query.set("decision_id", decision);
  } else {
    query.set("limit", "25");
    if (cursor) query.set("cursor", cursor);
  }
  const response = await fetch(`${api}/api/decision-journal?${query}`, {
    headers: { cookie: jar.toString() },
    cache: "no-store",
  });
  if (response.status === 401) redirect("/login");
  if (!response.ok) {
    return (
      <JournalUnavailable exact={decision !== ""} returnHref={returnHref} />
    );
  }
  const data = (await response.json()) as JournalResponse;
  if (
    decision &&
    (data.entries?.length !== 1 || data.entries[0]?.id !== decision)
  ) {
    return <JournalUnavailable exact returnHref={returnHref} />;
  }

  return (
    <main className="journal-page command-content-continuity">
      <AppPageHeader contentHeadingId="activity-page-title" />
      <p className="eyebrow">DECISION JOURNAL</p>
      <h1 id="activity-page-title">
        {decision
          ? "One immutable decision, directly linked."
          : "Every decision, in context."}
      </h1>
      <p className="lede">
        {decision
          ? "This durable owner-scoped view remains available after newer activity moves the record beyond the newest journal page."
          : "Review what each strategy proposed, how the deterministic risk gate responded, and what the non-live adapter recorded."}
      </p>
      <p className="journal-safety">
        <strong>READ-ONLY · LIVE EXECUTION UNAVAILABLE</strong>
        PAPER entries are simulations. SHADOW entries show what Arbion would
        have submitted. Neither mode sends a broker order.
      </p>

      <JournalList
        cursor={cursor}
        entries={data.entries ?? []}
        filter={filter}
        focused={decision !== ""}
      />

      {decision ? (
        <nav className="journal-pagination" aria-label="Exact record context">
          <Link href={returnHref}>← Return to journal context</Link>
          <span>Owner-scoped immutable record</span>
        </nav>
      ) : (
        <nav className="journal-pagination" aria-label="Decision journal pages">
          {cursor ? (
            <Link href={activityJournalHref({ filter })}>← Newest</Link>
          ) : (
            <span />
          )}
          {data.next_cursor ? (
            <Link
              href={activityJournalHref({
                cursor: data.next_cursor,
                filter,
              })}
            >
              Older entries →
            </Link>
          ) : (
            <span>End of journal</span>
          )}
        </nav>
      )}
    </main>
  );
}
