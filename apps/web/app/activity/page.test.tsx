import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const redirect = vi.hoisted(() =>
  vi.fn((path: string) => {
    throw new Error(`redirect:${path}`);
  }),
);
vi.mock("next/headers", () => ({
  cookies: async () => ({ toString: () => "session=test-only" }),
}));
vi.mock("next/navigation", () => ({ redirect }));
vi.mock("../app-page-header", () => ({
  AppPageHeader: () => <header>Shared navigation</header>,
}));

import Activity from "./page";
import type { JournalEntry } from "./journal-list";

const entry: JournalEntry = {
  id: "exact-record",
  created_at: "2026-09-11T12:00:00Z",
  strategy_instance_id: "instance",
  financial_account_id: "account",
  account_display_name: "Example portfolio",
  mandate_id: "mandate",
  mandate_version: 1,
  strategy_identifier: "ai_autonomous",
  execution_mode: "PAPER",
  strategy_state: "AI_MONITORING",
  source: "AI",
  decision_type: "ABSTAIN",
  structured_rationale: { reason: "Saved rationale" },
};
function mockResponse(entries: JournalEntry[], next_cursor = "") {
  const fetch = vi.fn().mockResolvedValue({
    status: 200,
    ok: true,
    json: async () => ({
      entries,
      next_cursor,
      live_execution_available: false,
    }),
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}
describe("Activity presentation and immutable route boundary", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("keeps the signed-out redirect and never substitutes local sample data", async () => {
    const fetch = vi.fn().mockResolvedValue({ status: 401, ok: false });
    vi.stubGlobal("fetch", fetch);
    await expect(
      Activity({ searchParams: Promise.resolve({}) }),
    ).rejects.toThrow("redirect:/login");
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("uses one owner-scoped no-store request and preserves cursor/filter pagination", async () => {
    const fetch = mockResponse([entry], "next opaque/cursor");
    render(
      await Activity({
        searchParams: Promise.resolve({
          cursor: "older cursor",
          view: "paper",
        }),
      }),
    );
    expect(
      screen.getByRole("heading", { level: 1, name: "Decision journal" }),
    ).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch).toHaveBeenCalledWith(
      expect.stringMatching(
        /\/api\/decision-journal\?limit=25&cursor=older\+cursor$/,
      ),
      { headers: { cookie: "session=test-only" }, cache: "no-store" },
    );
    expect(
      screen.getByRole("link", { name: "Older entries →" }),
    ).toHaveAttribute(
      "href",
      "/activity?cursor=next+opaque%2Fcursor&view=paper",
    );
    expect(screen.getByRole("link", { name: "← Newest" })).toHaveAttribute(
      "href",
      "/activity?view=paper",
    );
    expect(
      screen.getByText("READ-ONLY · LIVE EXECUTION UNAVAILABLE"),
    ).toBeInTheDocument();
  });

  it("keeps the exact record visible outside its return filter and fetches only its identity", async () => {
    const fetch = mockResponse([entry]);
    render(
      await Activity({
        searchParams: Promise.resolve({
          cursor: "older",
          view: "shadow",
          decision: entry.id,
        }),
      }),
    );
    expect(
      screen.getByRole("heading", { level: 1, name: "Decision record" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Example portfolio · Mandate v1"),
    ).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledWith(
      expect.stringMatching(
        /\/api\/decision-journal\?decision_id=exact-record$/,
      ),
      { headers: { cookie: "session=test-only" }, cache: "no-store" },
    );
    expect(
      screen.getByRole("link", { name: "← Return to journal context" }),
    ).toHaveAttribute("href", "/activity?cursor=older&view=shadow");
    expect(
      screen.queryByRole("navigation", { name: "Journal view filters" }),
    ).not.toBeInTheDocument();
  });

  it.each([
    { label: "missing", entries: [] },
    {
      label: "different identity",
      entries: [{ ...entry, id: "different-record" }],
    },
    { label: "ambiguous", entries: [entry, entry] },
  ])(
    "does not render a substitute for $label exact evidence",
    async ({ entries }) => {
      const fetch = mockResponse(entries);
      render(
        await Activity({
          searchParams: Promise.resolve({ decision: entry.id, view: "review" }),
        }),
      );
      expect(
        screen.getByRole("heading", { name: "Exact record unavailable" }),
      ).toBeInTheDocument();
      expect(
        screen.queryByRole("region", { name: "Decision journal entries" }),
      ).not.toBeInTheDocument();
      expect(
        screen.getByRole("link", { name: "Return to journal context" }),
      ).toHaveAttribute("href", "/activity?view=review");
      expect(fetch).toHaveBeenCalledTimes(1);
    },
  );

  it("keeps an unavailable response distinct from an empty journal", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ status: 503, ok: false }),
    );
    render(await Activity({ searchParams: Promise.resolve({}) }));
    expect(
      screen.getByRole("heading", { name: "Journal unavailable" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("Your journal is ready."),
    ).not.toBeInTheDocument();
  });
});
