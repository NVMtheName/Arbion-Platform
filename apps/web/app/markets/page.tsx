import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import type { MarketAccount } from "./market-command-surface";
import {
  safeMarketHealthHistory,
  type MarketHealthHistory,
} from "./market-health-contract";
import type { MarketSource } from "./market-source-grid";
import {
  emptyMarketWatchlist,
  type MarketWatchlistData,
} from "./market-watchlist";
import { MarketsWorkspace } from "./markets-workspace";

type SourcesResponse = {
  sources: MarketSource[];
  status_generated_at?: string;
  status_semantics?: "PROCESS_LOCAL_TIME_BOUNDED_PROVIDER_VERIFICATION";
  provider_errors_exposed?: false;
  live_execution_available: false;
};

export default async function MarketsPage() {
  const jar = await cookies();
  const api = process.env.API_BASE_URL ?? "http://localhost:8080";
  const [response, accountsResponse, historyResponse, watchlistResponse] =
    await Promise.all([
      fetch(`${api}/api/markets/sources`, {
        headers: { cookie: jar.toString() },
        cache: "no-store",
      }),
      fetch(`${api}/api/accounts`, {
        headers: { cookie: jar.toString() },
        cache: "no-store",
      }),
      fetch(`${api}/api/markets/source-history`, {
        headers: { cookie: jar.toString() },
        cache: "no-store",
      }).catch(() => undefined),
      fetch(`${api}/api/markets/watchlist`, {
        headers: { cookie: jar.toString() },
        cache: "no-store",
      }).catch(() => undefined),
    ]);
  if (response.status === 401) redirect("/login");
  const data: SourcesResponse = response.ok
    ? ((await response.json()) as SourcesResponse)
    : {
        sources: [],
        provider_errors_exposed: false,
        live_execution_available: false,
      };
  const accounts = accountsResponse.ok
    ? ((await accountsResponse.json()) as { accounts: MarketAccount[] })
        .accounts
    : [];
  let history: MarketHealthHistory | undefined;
  if (historyResponse?.ok) {
    try {
      history = safeMarketHealthHistory(await historyResponse.json());
    } catch {
      history = undefined;
    }
  }
  let watchlist: MarketWatchlistData = emptyMarketWatchlist;
  if (watchlistResponse?.ok) {
    try {
      const candidate = (await watchlistResponse.json()) as MarketWatchlistData;
      if (
        Array.isArray(candidate.items) &&
        candidate.items.length <= emptyMarketWatchlist.max_items &&
        candidate.max_items === emptyMarketWatchlist.max_items &&
        candidate.provider_write_available === false &&
        candidate.order_actions_available === false &&
        candidate.live_execution_available === false
      ) {
        watchlist = candidate;
      }
    } catch {
      watchlist = emptyMarketWatchlist;
    }
  }
  return (
    <MarketsWorkspace
      sources={data.sources}
      statusGeneratedAt={data.status_generated_at}
      accounts={accounts}
      history={history}
      watchlist={watchlist}
    />
  );
}
