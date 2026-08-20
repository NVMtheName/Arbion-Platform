import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import type { JournalEntry } from "../activity/journal-list";
import type { MarketSource } from "../markets/market-source-grid";
import { CommandCenterDashboard } from "./command-center-dashboard";
import { InsightPanel } from "./insight-panel";

type User = {
  email: string;
  display_name: string;
  entitlement: string;
  role: string;
};

type Preference = { connection_id: string; model_id: string };

export default async function Dashboard() {
  const cookieStore = await cookies();
  const api = process.env.API_BASE_URL ?? "http://localhost:8080";
  const headers = { cookie: cookieStore.toString() };
  const response = await fetch(`${api}/api/auth/me`, {
    headers,
    cache: "no-store",
  });
  if (!response.ok) redirect("/login");
  const { user } = (await response.json()) as { user: User };

  const [
    connectionsResponse,
    accountsResponse,
    preferenceResponse,
    sourcesResponse,
    journalResponse,
  ] = await Promise.all([
    fetch(`${api}/api/connections/ai`, { headers, cache: "no-store" }),
    fetch(`${api}/api/accounts`, { headers, cache: "no-store" }),
    fetch(`${api}/api/settings/neural-engine`, {
      headers,
      cache: "no-store",
    }),
    fetch(`${api}/api/markets/sources`, { headers, cache: "no-store" }),
    fetch(`${api}/api/decision-journal?limit=25`, {
      headers,
      cache: "no-store",
    }),
  ]);

  const connectionCount = connectionsResponse.ok
    ? ((await connectionsResponse.json()) as { connections: unknown[] })
        .connections.length
    : 0;
  const accountCount = accountsResponse.ok
    ? ((await accountsResponse.json()) as { accounts: unknown[] }).accounts
        .length
    : 0;
  const preference = preferenceResponse.ok
    ? ((await preferenceResponse.json()) as { preference: Preference | null })
        .preference
    : null;
  const sources = sourcesResponse.ok
    ? ((await sourcesResponse.json()) as { sources: MarketSource[] }).sources
    : [];
  const journalEntries = journalResponse.ok
    ? ((await journalResponse.json()) as { entries: JournalEntry[] }).entries
    : [];

  return (
    <CommandCenterDashboard
      accountCount={accountCount}
      asOf={new Date().toISOString()}
      connectionCount={connectionCount}
      journalEntries={journalEntries}
      sources={sources}
      user={user}
    >
      <InsightPanel
        configured={preference !== null}
        model={preference?.model_id}
      />
    </CommandCenterDashboard>
  );
}
