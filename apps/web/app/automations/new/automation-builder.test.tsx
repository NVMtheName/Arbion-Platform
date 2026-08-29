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

describe("AutomationBuilder AI non-live launch", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("directs a new owner to connect accounts and an AI decision provider", async () => {
    vi.stubGlobal("fetch", fetchFixtures());
    render(<AutomationBuilder />);
    expect(
      await screen.findByText(/connect coinbase or schwab first/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/connect and verify openai, claude, or gemini first/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /create ai paper draft/i }),
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

  it("offers an active Claude connection and only its approved shadow models", async () => {
    vi.stubGlobal(
      "fetch",
      fetchFixtures({
        ...connected,
        connections: [
          {
            id: "claude-1",
            display_name: "Claude",
            provider: "anthropic",
            status: "active",
          },
        ],
        preference: null,
        models: [
          { id: "claude-sonnet-5", display_name: "Claude Sonnet 5" },
          { id: "claude-unsupported", display_name: "Unsupported Claude" },
        ],
      }),
    );
    render(<AutomationBuilder />);
    expect(
      await screen.findByRole("option", { name: "Claude Sonnet 5" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("option", { name: "Unsupported Claude" }),
    ).not.toBeInTheDocument();
  });

  it("offers an active Gemini connection and only stable approved shadow models", async () => {
    vi.stubGlobal(
      "fetch",
      fetchFixtures({
        ...connected,
        connections: [
          {
            id: "gemini-1",
            display_name: "Gemini",
            provider: "gemini",
            status: "active",
          },
        ],
        preference: null,
        models: [
          { id: "gemini-3.7-flash", display_name: "Gemini 3.7 Flash" },
          { id: "gemini-3.1-pro-preview", display_name: "Preview model" },
        ],
      }),
    );
    render(<AutomationBuilder />);
    expect(
      await screen.findByRole("option", { name: "Gemini 3.7 Flash" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("option", { name: "Preview model" }),
    ).not.toBeInTheDocument();
  });

  it("creates an isolated Coinbase continuous AI Paper mandate by default", async () => {
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
      name: /create ai paper draft/i,
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
      execution_mode: "PAPER",
      options_allowed: false,
      ai_model_id: "gpt-5.6-sol",
      allowed_universe: { symbols: ["BTC", "ETH"] },
      schedule_conditions: { session: "CONTINUOUS" },
      risk_parameters: { max_trades_per_day: 6 },
    });
  });

  it("saves owner-selected deterministic guardrails with the draft", async () => {
    const fixtureFetch = fetchFixtures(connected);
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
        if (String(input) === "/api/automations" && init?.method === "POST") {
          return response({ automation: { id: "engine-guarded" } });
        }
        return fixtureFetch(input);
      }),
    );
    render(<AutomationBuilder />);
    fireEvent.change(await screen.findByLabelText(/allowed symbols/i), {
      target: { value: "BTC" },
    });
    fireEvent.change(screen.getByLabelText(/maximum paper actions/i), {
      target: { value: "4" },
    });
    fireEvent.change(screen.getByLabelText(/minimum cash reserve/i), {
      target: { value: "25" },
    });
    fireEvent.change(screen.getByLabelText(/maximum total account exposure/i), {
      target: { value: "500" },
    });
    fireEvent.change(
      screen.getByLabelText(/maximum one-symbol exposure \(usd\)/i),
      {
        target: { value: "100" },
      },
    );
    fireEvent.change(
      screen.getByLabelText(/maximum one-symbol concentration/i),
      {
        target: { value: "30" },
      },
    );
    const button = screen.getByRole("button", {
      name: /create ai paper draft/i,
    });
    await waitFor(() => expect(button).toBeEnabled());
    fireEvent.click(button);
    await screen.findByRole("link", { name: /review ai engine/i });
    const call = vi
      .mocked(fetch)
      .mock.calls.find(
        ([input, init]) =>
          String(input) === "/api/automations" && init?.method === "POST",
      );
    expect(JSON.parse(String(call?.[1]?.body)).risk_parameters).toEqual({
      max_trades_per_day: 4,
      minimum_cash_reserve: "25",
      max_capital_deployed: "500",
      max_single_position_amount: "100",
      max_single_position_percentage: "30",
    });
    expect(
      screen.getByText(/will not invent realized profit and loss/i),
    ).toBeInTheDocument();
  });

  it("blocks an invalid daily action or concentration limit in the browser", async () => {
    vi.stubGlobal("fetch", fetchFixtures(connected));
    render(<AutomationBuilder />);
    fireEvent.change(await screen.findByLabelText(/allowed symbols/i), {
      target: { value: "BTC" },
    });
    const button = screen.getByRole("button", {
      name: /create ai paper draft/i,
    });
    await waitFor(() => expect(button).toBeEnabled());
    fireEvent.change(screen.getByLabelText(/maximum paper actions/i), {
      target: { value: "49" },
    });
    expect(button).toBeDisabled();
    fireEvent.change(screen.getByLabelText(/maximum paper actions/i), {
      target: { value: "6" },
    });
    fireEvent.change(
      screen.getByLabelText(/maximum one-symbol concentration/i),
      {
        target: { value: "101" },
      },
    );
    expect(button).toBeDisabled();
  });

  it("shows the API's safe rejection reason", async () => {
    const fixtureFetch = fetchFixtures(connected);
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
        if (String(input) === "/api/automations" && init?.method === "POST") {
          return response(
            {
              error: {
                code: "NOT_FOUND",
                message: "The selected financial account could not be found.",
              },
            },
            false,
          );
        }
        return fixtureFetch(input);
      }),
    );
    render(<AutomationBuilder />);
    fireEvent.change(await screen.findByLabelText(/allowed symbols/i), {
      target: { value: "BTC" },
    });
    const button = screen.getByRole("button", {
      name: /create ai paper draft/i,
    });
    await waitFor(() => expect(button).toBeEnabled());
    fireEvent.click(button);
    expect(
      await screen.findByText(/selected financial account could not be found/i),
    ).toBeInTheDocument();
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
      await screen.findByLabelText(/new ai paper budget/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("option", { name: "Never touch" }),
    ).not.toBeInTheDocument();
  });

  it("keeps observation-only Shadow available as an explicit choice", async () => {
    const fixtureFetch = fetchFixtures(connected);
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
        if (String(input) === "/api/automations" && init?.method === "POST") {
          return response({ automation: { id: "shadow-1" } });
        }
        return fixtureFetch(input);
      }),
    );
    render(<AutomationBuilder />);
    fireEvent.change(await screen.findByLabelText(/ai learning mode/i), {
      target: { value: "SHADOW" },
    });
    fireEvent.change(screen.getByLabelText(/allowed symbols/i), {
      target: { value: "BTC" },
    });
    const button = screen.getByRole("button", {
      name: /create ai shadow draft/i,
    });
    await waitFor(() => expect(button).toBeEnabled());
    fireEvent.click(button);
    await screen.findByRole("link", { name: /review ai engine/i });
    const call = vi
      .mocked(fetch)
      .mock.calls.find(
        ([input, init]) =>
          String(input) === "/api/automations" && init?.method === "POST",
      );
    expect(JSON.parse(String(call?.[1]?.body)).execution_mode).toBe("SHADOW");
  });
});
