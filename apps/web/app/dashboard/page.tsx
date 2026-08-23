import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import {
  CommandCenterDashboard,
  type DashboardAccountSummary,
  type DashboardMoney,
} from "./command-center-dashboard";

type User = {
  email: string;
  display_name: string;
  entitlement: string;
  role: string;
};

type Preference = { connection_id: string; model_id: string };

type Account = {
  id: string;
  provider: string;
  display_name: string;
  status: string;
  last_synced_at?: string;
};

type Balances = {
  cash?: DashboardMoney;
  available_cash?: DashboardMoney;
  account_value?: DashboardMoney;
  equity?: DashboardMoney;
};

type CryptoPortfolio = {
  observed_value?: DashboardMoney;
  balances?: {
    cash?: DashboardMoney;
    available_cash?: DashboardMoney;
  };
  total_positions?: number;
  pricing_state?: "READY" | "PARTIAL" | "UNAVAILABLE";
  pricing_as_of?: string;
};

async function accountSummary(
  account: Account,
  base: string,
  headers: { cookie: string },
): Promise<DashboardAccountSummary> {
  const id = encodeURIComponent(account.id);
  const shared = {
    id: account.id,
    provider: account.provider,
    displayName: account.display_name,
    status: account.status,
    asOf: account.last_synced_at,
  };

  try {
    if (account.provider === "coinbase") {
      const response = await fetch(
        `${base}/api/accounts/${id}/portfolio/crypto`,
        {
          headers,
          cache: "no-store",
        },
      );
      if (!response.ok) {
        return { ...shared, availability: "unavailable" };
      }
      const portfolio = (
        (await response.json()) as { portfolio?: CryptoPortfolio }
      ).portfolio;
      if (!portfolio) return { ...shared, availability: "unavailable" };
      return {
        ...shared,
        observedValue: portfolio.observed_value,
        cash: portfolio.balances?.cash ?? portfolio.balances?.available_cash,
        positionCount: portfolio.total_positions,
        availability:
          portfolio.pricing_state === "READY"
            ? "ready"
            : portfolio.pricing_state === "PARTIAL"
              ? "partial"
              : "unavailable",
        asOf: portfolio.pricing_as_of ?? account.last_synced_at,
      };
    }

    const [balancesResponse, positionsResponse] = await Promise.all([
      fetch(`${base}/api/accounts/${id}/balances`, {
        headers,
        cache: "no-store",
      }),
      fetch(`${base}/api/accounts/${id}/positions`, {
        headers,
        cache: "no-store",
      }),
    ]);
    const balances = balancesResponse.ok
      ? ((await balancesResponse.json()) as { balances?: Balances }).balances
      : undefined;
    const positions = positionsResponse.ok
      ? ((await positionsResponse.json()) as { positions?: unknown[] })
          .positions
      : undefined;
    return {
      ...shared,
      observedValue: balances?.account_value ?? balances?.equity,
      cash: balances?.cash ?? balances?.available_cash,
      positionCount: positions?.length,
      availability:
        balancesResponse.ok && positionsResponse.ok
          ? "ready"
          : balancesResponse.ok || positionsResponse.ok
            ? "partial"
            : "unavailable",
    };
  } catch {
    return { ...shared, availability: "unavailable" };
  }
}

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

  const [connectionsResponse, accountsResponse, preferenceResponse] =
    await Promise.all([
      fetch(`${api}/api/connections/ai`, { headers, cache: "no-store" }),
      fetch(`${api}/api/accounts`, { headers, cache: "no-store" }),
      fetch(`${api}/api/settings/neural-engine`, {
        headers,
        cache: "no-store",
      }),
    ]);

  const connections = connectionsResponse.ok
    ? ((
        (await connectionsResponse.json()) as {
          connections?: { status?: string }[] | null;
        }
      ).connections ?? [])
    : [];
  const accounts = accountsResponse.ok
    ? ((
        (await accountsResponse.json()) as {
          accounts?: Account[] | null;
        }
      ).accounts ?? [])
    : [];
  const preference = preferenceResponse.ok
    ? (
        (await preferenceResponse.json()) as {
          preference: Preference | null;
        }
      ).preference
    : null;
  const activeAccounts = accounts.filter(
    (account) => account.status === "active",
  );
  const accountSummaries = await Promise.all(
    activeAccounts.map((account) => accountSummary(account, api, headers)),
  );

  return (
    <CommandCenterDashboard
      accounts={accountSummaries}
      connectionCount={
        connections.filter((connection) => connection.status === "active")
          .length
      }
      modelConfigured={preference !== null}
      modelID={preference?.model_id}
      user={user}
    />
  );
}
