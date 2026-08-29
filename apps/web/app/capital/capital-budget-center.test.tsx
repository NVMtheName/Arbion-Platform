import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: vi.fn() }),
}));

import { CapitalBudgetCenter } from "./capital-budget-center";

const accounts = [
  {
    id: "coinbase-account",
    display_name: "Coinbase Prime",
    provider: "coinbase",
    status: "active",
    currency: "USD",
  },
];

describe("CapitalBudgetCenter", () => {
  afterEach(cleanup);

  it("shows exact shared reservation totals and their strategy owners", () => {
    render(
      <CapitalBudgetCenter
        accounts={accounts}
        buckets={[
          {
            id: "bucket-btc",
            financialAccountID: "coinbase-account",
            name: "BTC sleeve",
            allocationType: "FIXED_AMOUNT",
            allocationValue: "250.0000000000",
            currency: "USD",
            protectedAmount: "0.0000000000",
            allocationLimit: "1000.0000000000",
            isReserve: false,
            status: "ACTIVE",
          },
          {
            id: "bucket-eth",
            financialAccountID: "coinbase-account",
            name: "ETH sleeve",
            allocationType: "FIXED_AMOUNT",
            allocationValue: "300.0000000000",
            currency: "USD",
            protectedAmount: "0.0000000000",
            allocationLimit: "1000.0000000000",
            isReserve: false,
            status: "ACTIVE",
          },
        ]}
        reservations={[
          {
            id: "reservation-btc",
            strategyInstanceID: "instance-btc",
            financialAccountID: "coinbase-account",
            capitalBucketID: "bucket-btc",
            executionMode: "SHADOW",
            reservationAmount: "250.0000000000",
            currency: "USD",
            reservationBasis: "BUCKET_FIXED_CAPACITY",
            accountAllocationLimit: "1000.0000000000",
            status: "ACTIVE",
            reservedAt: "2026-08-28T10:00:00Z",
          },
          {
            id: "reservation-eth",
            strategyInstanceID: "instance-eth",
            financialAccountID: "coinbase-account",
            capitalBucketID: "bucket-eth",
            executionMode: "SHADOW",
            reservationAmount: "300.0000000000",
            currency: "USD",
            reservationBasis: "BUCKET_FIXED_CAPACITY",
            accountAllocationLimit: "1000.0000000000",
            status: "ACTIVE",
            reservedAt: "2026-08-28T10:01:00Z",
          },
        ]}
        strategies={[
          {
            id: "instance-btc",
            automationMandateID: "mandate-btc",
            status: "ACTIVE",
            currentState: "AI_MONITORING",
          },
          {
            id: "instance-eth",
            automationMandateID: "mandate-eth",
            status: "ACTIVE",
            currentState: "AI_MONITORING",
          },
        ]}
      />,
    );

    const account = screen
      .getByRole("heading", { name: "Coinbase Prime" })
      .closest("section") as HTMLElement;
    expect(account).toHaveTextContent("Shared Shadow ceiling active");
    expect(account).toHaveTextContent("Fixed budgets defined$550");
    expect(account).toHaveTextContent("Shadow capital reserved$550");
    expect(account).toHaveTextContent("Paper starting cash$0");
    expect(account).toHaveTextContent("Shared Shadow ceiling$1,000");
    expect(account).toHaveTextContent("Unreserved Shadow headroom$450");
    expect(
      within(account).getAllByText("Reserved by an active strategy"),
    ).toHaveLength(2);
    expect(
      within(account).getByRole("link", {
        name: "Open BTC sleeve strategy →",
      }),
    ).toHaveAttribute("href", "/automations/mandate-btc");
    expect(
      screen.getByRole("region", { name: "Execution boundary" }),
    ).toHaveTextContent("neither grants order or live-execution permission");
  });

  it("explains exclusive claims and protected reserves without inventing headroom", () => {
    render(
      <CapitalBudgetCenter
        accounts={accounts}
        buckets={[
          {
            id: "exclusive",
            financialAccountID: "coinbase-account",
            name: "Main engine",
            allocationType: "FIXED_AMOUNT",
            allocationValue: "50.0000000000",
            currency: "USD",
            protectedAmount: "5.0000000000",
            isReserve: false,
            status: "ACTIVE",
          },
          {
            id: "reserve",
            financialAccountID: "coinbase-account",
            name: "Do not deploy",
            allocationType: "FIXED_AMOUNT",
            allocationValue: "100.0000000000",
            currency: "USD",
            protectedAmount: "100.0000000000",
            isReserve: true,
            status: "ACTIVE",
          },
        ]}
        reservations={[
          {
            id: "reservation",
            strategyInstanceID: "instance",
            financialAccountID: "coinbase-account",
            capitalBucketID: "exclusive",
            executionMode: "SHADOW",
            reservationAmount: "45.0000000000",
            currency: "USD",
            reservationBasis: "BUCKET_FIXED_CAPACITY",
            status: "ACTIVE",
            reservedAt: "2026-08-28T10:00:00Z",
          },
        ]}
        strategies={[]}
      />,
    );

    expect(screen.getByText("Exclusive Shadow claim")).toBeInTheDocument();
    expect(
      screen.getByText(/Shadow authority stays exclusive/i),
    ).toBeInTheDocument();
    expect(screen.getByText("Protected reserve")).toBeInTheDocument();
    expect(screen.getByText("Not applicable")).toBeInTheDocument();
    expect(screen.getByText("Exclusive when active")).toBeInTheDocument();
  });

  it("keeps simulated Paper cash separate from exclusive Shadow authority", () => {
    render(
      <CapitalBudgetCenter
        accounts={accounts}
        buckets={[
          {
            id: "shadow",
            financialAccountID: "coinbase-account",
            name: "Observed sleeve",
            allocationType: "FIXED_AMOUNT",
            allocationValue: "45.0000000000",
            currency: "USD",
            protectedAmount: "0.0000000000",
            isReserve: false,
            status: "ACTIVE",
          },
          {
            id: "paper",
            financialAccountID: "coinbase-account",
            name: "Paper lab",
            allocationType: "FIXED_AMOUNT",
            allocationValue: "500.0000000000",
            currency: "USD",
            protectedAmount: "0.0000000000",
            isReserve: false,
            status: "ACTIVE",
          },
        ]}
        reservations={[
          {
            id: "shadow-reservation",
            strategyInstanceID: "shadow-instance",
            financialAccountID: "coinbase-account",
            capitalBucketID: "shadow",
            executionMode: "SHADOW",
            reservationAmount: "45.0000000000",
            currency: "USD",
            reservationBasis: "BUCKET_FIXED_CAPACITY",
            status: "ACTIVE",
            reservedAt: "2026-08-28T10:00:00Z",
          },
          {
            id: "paper-reservation",
            strategyInstanceID: "paper-instance",
            financialAccountID: "coinbase-account",
            capitalBucketID: "paper",
            executionMode: "PAPER",
            reservationAmount: "500.0000000000",
            currency: "USD",
            reservationBasis: "PAPER_STARTING_CASH",
            status: "ACTIVE",
            reservedAt: "2026-08-28T10:01:00Z",
          },
        ]}
        strategies={[]}
      />,
    );

    const account = screen
      .getByRole("heading", { name: "Coinbase Prime" })
      .closest("section") as HTMLElement;
    expect(account).toHaveTextContent("Exclusive Shadow claim");
    expect(account).toHaveTextContent("Shadow capital reserved$45");
    expect(account).toHaveTextContent("Paper starting cash$500");
    expect(account).toHaveTextContent(/Paper simulations may run in parallel/i);
    expect(account).toHaveTextContent(
      /Paper starting cash is simulated and excluded from Shadow/i,
    );
    expect(account).toHaveTextContent("Simulation-only");
    expect(account).toHaveTextContent("Used by an active Paper simulation");
  });

  it("fails closed when the complete inventory cannot be loaded", () => {
    render(
      <CapitalBudgetCenter
        accounts={accounts}
        buckets={[]}
        reservations={[]}
        strategies={[]}
        inventoryAvailable={false}
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent(
      "not showing partial totals",
    );
    expect(screen.queryByText("Connected accounts")).not.toBeInTheDocument();
  });

  it("does not calculate totals from malformed decimal evidence", () => {
    render(
      <CapitalBudgetCenter
        accounts={accounts}
        buckets={[
          {
            id: "malformed",
            financialAccountID: "coinbase-account",
            name: "Malformed evidence",
            allocationType: "FIXED_AMOUNT",
            allocationValue: "not-a-decimal",
            currency: "USD",
            protectedAmount: "0.0000000000",
            isReserve: false,
            status: "ACTIVE",
          },
        ]}
        reservations={[]}
        strategies={[]}
      />,
    );

    expect(screen.getByText("Capital inventory invalid")).toBeInTheDocument();
    expect(screen.getAllByText("Unavailable")).toHaveLength(3);
    expect(
      screen.getByText(/will not calculate account totals/),
    ).toBeInTheDocument();
  });
});
