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
  return { ok, json: async () => payload } as Response;
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
    if (url.includes("/models"))
      return response({ models: fixtures.models ?? [] });
    if (url.includes("settings/neural-engine"))
      return response({ preference: fixtures.preference ?? null });
    if (url.includes("capital-buckets"))
      return response({ capital_buckets: fixtures.buckets ?? [] });
    if (url.includes("connections/ai"))
      return response({ connections: fixtures.connections ?? [] });
    return response({ accounts: fixtures.accounts ?? [] });
  });
}

const connected: Fixtures = {
  accounts: [
    {
      id: "account-1",
      display_name: "Coinbase",
      provider: "coinbase",
      status: "active",
    },
  ],
  buckets: [
    {
      id: "budget-1",
      name: "$1 AI test",
      status: "ACTIVE",
      is_reserve: false,
      financial_account_id: "account-1",
    },
  ],
  connections: [
    {
      id: "ai-1",
      display_name: "OpenAI",
      provider: "openai",
      status: "active",
    },
  ],
  preference: { connection_id: "ai-1", model_id: "gpt-5.6-sol" },
  models: [
    { id: "gpt-4.1", display_name: "Unsupported" },
    { id: "gpt-5.6-sol", display_name: "GPT-5.6 Sol" },
  ],
};

describe("AutomationBuilder AI Shadow launch", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("directs a new owner to connect accounts and OpenAI", async () => {
    vi.stubGlobal("fetch", fetchFixtures());
    render(<AutomationBuilder />);
    expect(
      await screen.findByText(/connect coinbase or schwab first/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/connect and verify openai first/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /create ai shadow draft/i }),
    ).toBeDisabled();
  });

  it("shows only approved Arbion shadow models", async () => {
    vi.stubGlobal("fetch", fetchFixtures(connected));
    render(<AutomationBuilder />);
    expect(
      await screen.findByRole("option", { name: "GPT-5.6 Sol" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("option", { name: "Unsupported" }),
    ).not.toBeInTheDocument();
  });

  it("creates an isolated Coinbase continuous AI Shadow mandate", async () => {
    const fixtureFetch = fetchFixtures(connected);
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
        if (String(input) === "/api/automations" && init?.method === "POST") {
          return response({ automation: { id: "engine-1" } });
        }
        return fixtureFetch(input);
      }),
    );
    render(<AutomationBuilder />);
    fireEvent.change(await screen.findByLabelText(/allowed symbols/i), {
      target: { value: "BTC, ETH" },
    });
    fireEvent.click(
      screen.getByLabelText(/run guarded evaluations automatically/i),
    );
    const button = screen.getByRole("button", {
      name: /create ai shadow draft/i,
    });
    await waitFor(() => expect(button).toBeEnabled());
    fireEvent.click(button);
    expect(
      await screen.findByRole("link", { name: /review ai engine/i }),
    ).toHaveAttribute("href", "/automations/engine-1");
    const call = vi
      .mocked(fetch)
      .mock.calls.find(
        ([input, init]) =>
          String(input) === "/api/automations" && init?.method === "POST",
      );
    expect(JSON.parse(String(call?.[1]?.body))).toMatchObject({
      automation_type: "AI_AUTONOMOUS",
      strategy_identifier: null,
      autonomy_level: "FULL_AUTONOMOUS",
      execution_mode: "SHADOW",
      options_allowed: false,
      ai_model_id: "gpt-5.6-sol",
      allowed_universe: { symbols: ["BTC", "ETH"] },
      schedule_conditions: { session: "CONTINUOUS" },
    });
  });

  it("keeps reserve buckets unavailable", async () => {
    vi.stubGlobal(
      "fetch",
      fetchFixtures({
        ...connected,
        buckets: [
          {
            id: "reserve",
            name: "Never touch",
            status: "ACTIVE",
            is_reserve: true,
            financial_account_id: "account-1",
          },
        ],
      }),
    );
    render(<AutomationBuilder />);
    expect(
      await screen.findByLabelText(/new ai shadow budget/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("option", { name: "Never touch" }),
    ).not.toBeInTheDocument();
  });
});
