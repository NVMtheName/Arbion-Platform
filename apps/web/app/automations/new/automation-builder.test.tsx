import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import AutomationBuilder from "./automation-builder";

function response(payload: unknown, ok = true) {
  return {
    ok,
    json: async () => payload,
  } as Response;
}

type Fixtures = {
  accounts?: Record<string, unknown>[];
  buckets?: Record<string, unknown>[];
  connections?: Record<string, unknown>[];
  preference?: Record<string, unknown> | null;
  models?: Record<string, unknown>[];
};

function fetchFixtures(fixtures: Fixtures = {}) {
  return vi.fn(async (input: string | URL | Request) => {
    const url = String(input);
    if (url.includes("/models")) {
      return response({ models: fixtures.models ?? [] });
    }
    if (url.includes("settings/neural-engine")) {
      return response({ preference: fixtures.preference ?? null });
    }
    if (url.includes("capital-buckets")) {
      return response({ capital_buckets: fixtures.buckets ?? null });
    }
    if (url.includes("connections/ai")) {
      return response({ connections: fixtures.connections ?? null });
    }
    return response({ accounts: fixtures.accounts ?? null });
  });
}

describe("AutomationBuilder strategy launch", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("directs a new user to connect the required account and AI provider", async () => {
    vi.stubGlobal("fetch", fetchFixtures());

    render(<AutomationBuilder />);

    expect(
      await screen.findByText(/connect a financial account first/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/connect an AI provider first/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /create strategy draft/i }),
    ).toBeDisabled();
    expect(
      screen.getAllByRole("link", { name: /open connection hub/i })[0],
    ).toHaveAttribute("href", "/connections#financial-accounts");
  });

  it("keeps protected reserves out of the selectable trading budget", async () => {
    vi.stubGlobal(
      "fetch",
      fetchFixtures({
        accounts: [
          {
            id: "account-1",
            display_name: "Schwab account",
            status: "active",
          },
        ],
        buckets: [
          {
            ID: "reserve-1",
            Name: "Never touch",
            Status: "ACTIVE",
            IsReserve: true,
            FinancialAccountID: "account-1",
          },
        ],
      }),
    );

    render(<AutomationBuilder />);

    expect(
      await screen.findByLabelText(/amount available to this strategy/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("option", { name: "Never touch" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /create strategy draft/i }),
    ).toBeDisabled();
  });

  it("offers only budgets owned by the selected financial account", async () => {
    vi.stubGlobal(
      "fetch",
      fetchFixtures({
        accounts: [
          {
            id: "account-1",
            display_name: "First account",
            status: "active",
          },
          {
            id: "account-2",
            display_name: "Second account",
            status: "active",
          },
        ],
        buckets: [
          {
            ID: "bucket-1",
            Name: "First budget",
            Status: "ACTIVE",
            IsReserve: false,
            FinancialAccountID: "account-1",
          },
          {
            ID: "bucket-2",
            Name: "Second budget",
            Status: "ACTIVE",
            IsReserve: false,
            FinancialAccountID: "account-2",
          },
        ],
      }),
    );

    render(<AutomationBuilder />);

    expect(
      await screen.findByRole("option", { name: "First budget" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("option", { name: "Second budget" }),
    ).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText(/financial account/i), {
      target: { value: "account-2" },
    });

    expect(
      screen.getByRole("option", { name: "Second budget" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("option", { name: "First budget" }),
    ).not.toBeInTheDocument();
  });

  it("uses the saved AI preference and creates a non-executing strategy draft", async () => {
    const fixtureFetch = fetchFixtures({
      accounts: [
        {
          id: "account-1",
          display_name: "Schwab brokerage",
          status: "active",
        },
      ],
      buckets: [
        {
          id: "budget-1",
          name: "$1 wheel test",
          status: "ACTIVE",
          is_reserve: false,
          financial_account_id: "account-1",
        },
      ],
      connections: [
        {
          id: "ai-1",
          display_name: "OpenAI",
          status: "active",
        },
      ],
      preference: { connection_id: "ai-1", model_id: "gpt-5" },
      models: [
        { id: "gpt-5-mini", display_name: "GPT-5 mini" },
        { id: "gpt-5", display_name: "GPT-5" },
      ],
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
        if (String(input) === "/api/automations" && init?.method === "POST") {
          return response({ automation: { id: "strategy-1" } });
        }
        return fixtureFetch(input);
      }),
    );

    render(<AutomationBuilder />);

    expect(
      await screen.findByRole("option", { name: "GPT-5" }),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByLabelText(/ai model/i)).toHaveValue("gpt-5"),
    );
    const createButton = screen.getByRole("button", {
      name: /create strategy draft/i,
    });
    await waitFor(() => expect(createButton).toBeEnabled());

    fireEvent.click(createButton);

    expect(
      await screen.findByRole("link", { name: /review strategy/i }),
    ).toHaveAttribute("href", "/automations/strategy-1");
    const postCall = vi
      .mocked(fetch)
      .mock.calls.find(
        ([input, init]) =>
          String(input) === "/api/automations" && init?.method === "POST",
      );
    const body = JSON.parse(String(postCall?.[1]?.body));
    expect(body).toMatchObject({
      financial_account_id: "account-1",
      automation_type: "HYBRID",
      strategy_identifier: "wheel",
      ai_provider_connection_id: "ai-1",
      ai_model_id: "gpt-5",
      capital_bucket_id: "budget-1",
      autonomy_level: "CONFIRM_EACH",
      execution_mode: "PAPER",
    });
  });

  it("creates the first trading budget without leaving strategy setup", async () => {
    const fixtureFetch = fetchFixtures({
      accounts: [
        {
          id: "coinbase-1",
          display_name: "Coinbase Advanced",
          status: "active",
        },
      ],
      connections: [
        {
          id: "ai-1",
          display_name: "OpenAI",
          status: "active",
        },
      ],
      models: [{ id: "gpt-5", display_name: "GPT-5" }],
    });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
        if (
          String(input) === "/api/capital-buckets" &&
          init?.method === "POST"
        ) {
          return response({
            capital_bucket: {
              id: "budget-1",
              name: "Wheel budget",
              status: "ACTIVE",
              is_reserve: false,
              financial_account_id: "coinbase-1",
            },
          });
        }
        return fixtureFetch(input);
      }),
    );

    render(<AutomationBuilder />);

    const amount = await screen.findByLabelText(
      /amount available to this strategy/i,
    );
    fireEvent.change(amount, { target: { value: "100" } });
    fireEvent.click(
      screen.getByRole("button", { name: /save trading budget/i }),
    );

    expect(
      await screen.findByText(/trading budget saved for this account/i),
    ).toBeInTheDocument();
    expect(screen.getByLabelText(/^trading budget$/i)).toHaveValue("budget-1");
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: /create strategy draft/i }),
      ).toBeEnabled(),
    );
    const budgetCall = vi
      .mocked(fetch)
      .mock.calls.find(
        ([input, init]) =>
          String(input) === "/api/capital-buckets" && init?.method === "POST",
      );
    expect(JSON.parse(String(budgetCall?.[1]?.body))).toMatchObject({
      financial_account_id: "coinbase-1",
      allocation_type: "FIXED_AMOUNT",
      allocation_value: "100",
      protected_amount: "0",
      is_reserve: false,
    });
  });
});
