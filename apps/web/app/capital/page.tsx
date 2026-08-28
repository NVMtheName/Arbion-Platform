import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { AppPageHeader } from "../app-page-header";
import {
  CapitalBudgetCenter,
  type CapitalAccount,
  type CapitalBucketView,
  type CapitalReservationView,
  type CapitalStrategyView,
} from "./capital-budget-center";

type RecordValue = Record<string, unknown>;

function text(record: RecordValue, key: string, legacy: string) {
  const value = record[key] ?? record[legacy];
  return typeof value === "string" ? value : "";
}

function optionalText(record: RecordValue, key: string, legacy: string) {
  const value = text(record, key, legacy);
  return value || undefined;
}

function flag(record: RecordValue, key: string, legacy: string) {
  const value = record[key] ?? record[legacy];
  return typeof value === "boolean" ? value : false;
}

async function inventory<T>(
  url: string,
  headers: { cookie: string },
  key: string,
): Promise<{ available: boolean; unauthorized: boolean; items: T[] }> {
  try {
    const response = await fetch(url, { headers, cache: "no-store" });
    if (response.status === 401)
      return { available: false, unauthorized: true, items: [] };
    if (!response.ok)
      return { available: false, unauthorized: false, items: [] };
    const payload = (await response.json()) as Record<string, unknown>;
    return {
      available: true,
      unauthorized: false,
      items: Array.isArray(payload[key]) ? (payload[key] as T[]) : [],
    };
  } catch {
    return { available: false, unauthorized: false, items: [] };
  }
}

export default async function CapitalPage() {
  const jar = await cookies();
  const headers = { cookie: jar.toString() };
  const base = process.env.API_BASE_URL ?? "http://localhost:8080";
  const [accountsResult, bucketsResult, reservationsResult, strategiesResult] =
    await Promise.all([
      inventory<RecordValue>(`${base}/api/accounts`, headers, "accounts"),
      inventory<RecordValue>(
        `${base}/api/capital-buckets`,
        headers,
        "capital_buckets",
      ),
      inventory<RecordValue>(
        `${base}/api/strategy-capital-reservations`,
        headers,
        "capital_reservations",
      ),
      inventory<RecordValue>(
        `${base}/api/strategy-instances`,
        headers,
        "strategy_instances",
      ),
    ]);

  if (
    accountsResult.unauthorized ||
    bucketsResult.unauthorized ||
    reservationsResult.unauthorized ||
    strategiesResult.unauthorized
  )
    redirect("/login");

  const accounts: CapitalAccount[] = accountsResult.items.map((account) => ({
    id: text(account, "id", "ID"),
    display_name:
      text(account, "display_name", "DisplayName") || "Financial account",
    provider: text(account, "provider", "Provider"),
    status: text(account, "status", "Status"),
    currency: text(account, "base_currency", "BaseCurrency")
      .trim()
      .toUpperCase(),
  }));
  const buckets: CapitalBucketView[] = bucketsResult.items.map((bucket) => ({
    id: text(bucket, "id", "ID"),
    financialAccountID: text(
      bucket,
      "financial_account_id",
      "FinancialAccountID",
    ),
    name: text(bucket, "name", "Name") || "Trading budget",
    allocationType: text(bucket, "allocation_type", "AllocationType"),
    allocationValue: text(bucket, "allocation_value", "AllocationValue"),
    currency: text(bucket, "currency", "Currency").trim().toUpperCase(),
    protectedAmount: text(bucket, "protected_amount", "ProtectedAmount") || "0",
    allocationLimit: optionalText(
      bucket,
      "allocation_limit",
      "AllocationLimit",
    ),
    isReserve: flag(bucket, "is_reserve", "IsReserve"),
    status: text(bucket, "status", "Status"),
  }));
  const reservations: CapitalReservationView[] = reservationsResult.items.map(
    (reservation) => ({
      id: text(reservation, "id", "ID"),
      strategyInstanceID: text(
        reservation,
        "strategy_instance_id",
        "StrategyInstanceID",
      ),
      financialAccountID: text(
        reservation,
        "financial_account_id",
        "FinancialAccountID",
      ),
      capitalBucketID: text(
        reservation,
        "capital_bucket_id",
        "CapitalBucketID",
      ),
      executionMode: text(reservation, "execution_mode", "ExecutionMode"),
      reservationAmount: optionalText(
        reservation,
        "reservation_amount",
        "ReservationAmount",
      ),
      currency: text(reservation, "currency", "Currency").trim().toUpperCase(),
      reservationBasis: text(
        reservation,
        "reservation_basis",
        "ReservationBasis",
      ),
      accountAllocationLimit: optionalText(
        reservation,
        "account_allocation_limit",
        "AccountAllocationLimit",
      ),
      status: text(reservation, "status", "Status"),
      reservedAt: text(reservation, "reserved_at", "ReservedAt"),
      releasedAt: optionalText(reservation, "released_at", "ReleasedAt"),
    }),
  );
  const strategies: CapitalStrategyView[] = strategiesResult.items.map(
    (strategy) => ({
      id: text(strategy, "id", "ID"),
      automationMandateID: text(
        strategy,
        "automation_mandate_id",
        "AutomationMandateID",
      ),
      status: text(strategy, "status", "Status"),
      currentState: text(strategy, "current_state", "CurrentState"),
    }),
  );

  return (
    <main className="connections-page capital-center-page">
      <AppPageHeader />
      <section className="capital-center-hero">
        <div>
          <p className="eyebrow">CAPITAL CONTROL CENTER</p>
          <h1>Every strategy gets a boundary.</h1>
          <p className="lede">
            See exactly what Arbion has assigned, protected, and reserved across
            Coinbase and Schwab—before any strategy is allowed to act.
          </p>
        </div>
        <div className="capital-center-assurance">
          <span>LIVE EXECUTION</span>
          <strong>Unavailable</strong>
          <p>No screen here can place or authorize a broker order.</p>
        </div>
      </section>
      <CapitalBudgetCenter
        accounts={accounts}
        buckets={buckets}
        reservations={reservations}
        strategies={strategies}
        inventoryAvailable={
          accountsResult.available &&
          bucketsResult.available &&
          reservationsResult.available &&
          strategiesResult.available
        }
      />
    </main>
  );
}
