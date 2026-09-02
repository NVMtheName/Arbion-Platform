import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import {
  activityJournalHref,
  JournalList,
  normalizeJournalFilter,
  projectDecisionReviewIndex,
  type JournalEntry,
} from "./journal-list";

const entry: JournalEntry = {
  id: "decision-1",
  created_at: "2026-08-18T18:00:00Z",
  strategy_instance_id: "instance-1",
  financial_account_id: "account-1",
  account_display_name: "Schwab Brokerage",
  mandate_id: "mandate-1",
  mandate_version: 2,
  strategy_identifier: "wheel",
  execution_mode: "PAPER",
  strategy_state: "READY_FOR_PUT",
  resulting_state: "SHORT_PUT_OPEN",
  source: "STRATEGY",
  decision_type: "ALLOW_SIMULATED_FILLED",
  structured_rationale: { candidate_count: 4, reason: "candidate_selected" },
  risk_decision: "ALLOW",
  approval_required: false,
  risk_reason_codes: ["ALLOWED"],
  execution_status: "SIMULATED_FILLED",
  symbol: "AAPL",
  instrument: "OPTION",
  side: "SELL_TO_OPEN",
  quantity: "1.0000000000",
  price: "1.2500000000",
  notional: "125.0000000000",
};

describe("Decision Journal", () => {
  afterEach(cleanup);

  it("renders useful decision evidence with an explicit non-live boundary", () => {
    render(<JournalList entries={[entry]} />);

    expect(screen.getByText("Wheel · AAPL")).toBeInTheDocument();
    expect(screen.getByText("Risk Allow")).toBeInTheDocument();
    expect(screen.getByText("Simulated Filled")).toBeInTheDocument();
    expect(screen.getByText("$125.00")).toBeInTheDocument();
    expect(
      screen.getByText(
        "Simulation evidence only — no real broker order was sent.",
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Review immutable details").closest("details"),
    ).not.toHaveAttribute("open");
  });

  it("guides the owner when there are no recorded decisions", () => {
    render(<JournalList entries={[]} />);

    expect(screen.getByText("Your journal is ready.")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "View automations" }),
    ).toHaveAttribute("href", "/automations");
  });

  it("compares the newest and prior AI conclusions without rerunning the model", () => {
    const latest: JournalEntry = {
      ...entry,
      id: "decision-latest",
      strategy_identifier: "ai_shadow",
      source: "AI",
      decision_type: "ALLOW_SIMULATED_FILLED",
      created_at: "2026-08-30T11:31:52Z",
      structured_rationale: {
        ai_provider: "openai",
        model_id: "gpt-5.6-sol",
        profile: "deep",
        market_observed_at: "2026-08-30T11:31:50Z",
        input_evidence: {
          provider: "coinbase",
          markets: [
            {
              symbol: "BTC",
              feed: "rest_ticker",
              quality: "REAL_TIME_SINGLE_VENUE",
              observed_at: "2026-08-30T11:31:50Z",
            },
            {
              symbol: "ETH",
              feed: "rest_ticker",
              quality: "REAL_TIME_SINGLE_VENUE",
              observed_at: "2026-08-30T11:31:49Z",
            },
          ],
        },
      },
    };
    const previous: JournalEntry = {
      ...latest,
      id: "decision-previous",
      decision_type: "ABSTAIN",
      created_at: "2026-08-30T10:31:22Z",
    };

    render(<JournalList entries={[previous, latest]} />);

    expect(
      screen.getByRole("heading", {
        name: "Compare each AI engine’s newest conclusion.",
      }),
    ).toBeInTheDocument();
    expect(screen.getByText("Conclusion changed")).toBeInTheDocument();
    expect(screen.getByText("OpenAI · gpt-5.6-sol · Deep")).toBeInTheDocument();
    expect(screen.getByText("Coinbase · BTC · ETH")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Open newest record ↓" }),
    ).toHaveAttribute("href", "#decision-decision-latest");
    expect(
      screen.getByText(/never reruns a model, calls a financial provider/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Provenance + market evidence").closest("details"),
    ).not.toHaveAttribute("open");
  });

  it("keeps Paper and Shadow evidence distinct and fails provenance closed", () => {
    const paper: JournalEntry = {
      ...entry,
      id: "paper-decision",
      strategy_instance_id: "paper-instance",
      strategy_identifier: "ai_shadow",
      source: "AI",
      execution_mode: "PAPER",
      structured_rationale: { ai_provider: "openai" },
    };
    const shadow: JournalEntry = {
      ...paper,
      id: "shadow-decision",
      strategy_instance_id: "shadow-instance",
      execution_mode: "SHADOW",
      account_display_name: "Coinbase Shadow",
      created_at: "2026-08-18T19:00:00Z",
    };

    const projected = projectDecisionReviewIndex([paper, shadow]);
    expect(projected).toHaveLength(2);
    expect(projected.map((item) => item.executionMode).sort()).toEqual([
      "PAPER",
      "SHADOW",
    ]);
    expect(
      projected.every((item) => item.provenanceStatus === "UNAVAILABLE"),
    ).toBe(true);

    render(<JournalList entries={[paper, shadow]} />);
    expect(screen.getAllByText("Unavailable — not inferred")).toHaveLength(6);
    for (const summary of screen.getAllByText("Provenance + market evidence")) {
      expect(summary.closest("details")).toHaveAttribute("open");
    }
    expect(
      screen.getAllByText(/Paper and Shadow remain non-live/i).length,
    ).toBeGreaterThan(0);
  });

  it("automatically opens immutable detail when a decision needs review", () => {
    render(
      <JournalList
        entries={[
          {
            ...entry,
            risk_decision: "DENY",
            execution_status: "RISK_DENIED",
            risk_reason_codes: ["CAPITAL_LIMIT_EXCEEDED"],
          },
        ]}
      />,
    );

    expect(
      screen.getByText("Review immutable details").closest("details"),
    ).toHaveAttribute("open");
    expect(screen.getByText("Review required")).toBeInTheDocument();
    expect(screen.getByText("Capital Limit Exceeded")).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "Resolve account evidence" }),
    ).not.toBeInTheDocument();
  });

  it("links an exact reconciliation denial to its account resolution evidence", () => {
    render(
      <JournalList
        entries={[
          {
            ...entry,
            risk_decision: "DENY",
            execution_status: "RISK_DENIED",
            risk_reason_codes: ["RECONCILIATION_DRIFT_DETECTED"],
          },
        ]}
      />,
    );

    expect(
      screen.getByRole("link", { name: "Resolve account evidence" }),
    ).toHaveAttribute(
      "href",
      "/accounts/account-1#reconciliation-resolution-title",
    );
  });

  it("filters the current immutable page by mode and review state", () => {
    const shadow: JournalEntry = {
      ...entry,
      id: "decision-shadow",
      account_display_name: "Coinbase Shadow",
      execution_mode: "SHADOW",
      risk_decision: "DENY",
      execution_status: "RISK_DENIED",
      risk_reason_codes: ["RECONCILIATION_DRIFT_DETECTED"],
      symbol: "BTC",
    };

    const { rerender } = render(
      <JournalList cursor="opaque cursor" entries={[entry, shadow]} />,
    );

    expect(screen.getByRole("link", { name: "All records 2" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("link", { name: "Paper 1" })).toHaveAttribute(
      "href",
      "/activity?cursor=opaque+cursor&view=paper",
    );

    rerender(
      <JournalList
        cursor="opaque cursor"
        entries={[entry, shadow]}
        filter="PAPER"
      />,
    );
    expect(screen.getByText("Wheel · AAPL")).toBeInTheDocument();
    expect(screen.queryByText("Wheel · BTC")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Paper 1" })).toHaveAttribute(
      "aria-current",
      "page",
    );

    rerender(
      <JournalList
        cursor="opaque cursor"
        entries={[entry, shadow]}
        filter="NEEDS_REVIEW"
      />,
    );
    expect(screen.queryByText("Wheel · AAPL")).not.toBeInTheDocument();
    expect(screen.getByText("Wheel · BTC")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Resolve account evidence" }),
    ).toHaveAttribute(
      "href",
      "/accounts/account-1#reconciliation-resolution-title",
    );
  });

  it("explains an empty filtered view without hiding saved history", () => {
    render(
      <JournalList
        cursor="older-page"
        entries={[entry]}
        filter="NEEDS_REVIEW"
      />,
    );

    expect(screen.getByText("This journal view is clear.")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Show all records" }),
    ).toHaveAttribute("href", "/activity?cursor=older-page");
    expect(screen.queryByText("Wheel · AAPL")).not.toBeInTheDocument();
  });

  it("normalizes shareable journal views and preserves opaque cursors", () => {
    expect(normalizeJournalFilter("paper")).toBe("PAPER");
    expect(normalizeJournalFilter("shadow")).toBe("SHADOW");
    expect(normalizeJournalFilter("review")).toBe("NEEDS_REVIEW");
    expect(normalizeJournalFilter("unexpected")).toBe("ALL");
    expect(normalizeJournalFilter()).toBe("ALL");
    expect(
      activityJournalHref({
        cursor: "time:2026-09-02T10:00:35Z/id value",
        filter: "SHADOW",
      }),
    ).toBe(
      "/activity?cursor=time%3A2026-09-02T10%3A00%3A35Z%2Fid+value&view=shadow",
    );
    expect(activityJournalHref({ filter: "NEEDS_REVIEW" })).toBe(
      "/activity?view=review",
    );
    expect(activityJournalHref({ filter: "ALL" })).toBe("/activity");
  });
});
