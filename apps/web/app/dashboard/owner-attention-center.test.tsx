import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import {
  OwnerAttentionCenter,
  type OwnerAttentionOverview,
} from "./owner-attention-center";

const clearAttention: OwnerAttentionOverview = {
  generated_at: "2026-08-28T02:30:00Z",
  status: "CLEAR",
  items: [],
  total: 0,
  attention_count: 0,
  stopped_count: 0,
  live_execution_available: false,
  broker_action_requested: false,
};

describe("Owner Attention Center", () => {
  afterEach(cleanup);

  it("shows a truthful clear state without implying broker capability", () => {
    render(<OwnerAttentionCenter attention={clearAttention} available />);

    const region = screen.getByRole("region", {
      name: "What needs you now.",
    });
    expect(region).toHaveTextContent("No active issues.");
    expect(region).toHaveTextContent("All monitored controls clear");
    expect(region).toHaveTextContent("Read-only status");
    expect(region).toHaveTextContent("No credentials");
    expect(region).toHaveTextContent("broker actions");
  });

  it("uses fixed safe copy and routes each active condition to review", () => {
    render(
      <OwnerAttentionCenter
        available
        attention={{
          ...clearAttention,
          status: "STOPPED",
          total: 3,
          attention_count: 2,
          stopped_count: 1,
          items: [
            {
              id: "stop-1",
              code: "OWNER_SAFETY_STOP",
              severity: "STOPPED",
              resource_type: "OWNER",
              occurred_at: "2026-08-28T02:30:00Z",
              count: 1,
            },
            {
              id: "schedule-1",
              code: "SCHEDULE_FAILURE",
              severity: "ATTENTION",
              resource_type: "AUTOMATION",
              resource_id: "mandate-1",
              occurred_at: "2026-08-28T02:20:00Z",
              count: 2,
            },
            {
              id: "drift-1",
              code: "PORTFOLIO_DRIFT_REVIEW_REQUIRED",
              severity: "ATTENTION",
              resource_type: "ACCOUNT",
              resource_id: "account-1",
              occurred_at: "2026-08-28T02:10:00Z",
              count: 1,
            },
          ],
        }}
      />,
    );

    const region = screen.getByRole("region", {
      name: "What needs you now.",
    });
    expect(region).toHaveTextContent("Owner safety stop is active");
    expect(region).toHaveTextContent("Scheduled evaluation needs attention");
    expect(region).toHaveTextContent("2 consecutive failed cycles");
    expect(region).toHaveTextContent("Portfolio inventory changed");
    expect(
      screen.getByRole("link", { name: /review risk controls/i }),
    ).toHaveAttribute("href", "/settings/risk");
    expect(
      screen.getByRole("link", { name: /review automation/i }),
    ).toHaveAttribute("href", "/automations/mandate-1");
    expect(
      screen.getByRole("link", { name: /review account/i }),
    ).toHaveAttribute(
      "href",
      "/accounts/account-1#reconciliation-resolution-title",
    );
    expect(region).not.toHaveTextContent("provider response");
    expect(region).not.toHaveTextContent("private stop reason");
    expect(region).not.toHaveTextContent("quantity");
  });

  it("fails visibly closed when attention evidence is unavailable", () => {
    render(<OwnerAttentionCenter available={false} />);

    const region = screen.getByRole("region", {
      name: "What needs you now.",
    });
    expect(region).toHaveTextContent("Attention status is unavailable.");
    expect(region).toHaveTextContent("not inferring a healthy state");
    expect(region).not.toHaveTextContent("No active issues.");
  });

  it("routes Schwab quote-quality attention directly to its readiness proof", () => {
    render(
      <OwnerAttentionCenter
        available
        attention={{
          ...clearAttention,
          status: "ATTENTION",
          total: 1,
          attention_count: 1,
          items: [
            {
              id: "schwab-schedule-1",
              code: "SCHWAB_MARKET_DATA_ATTENTION",
              severity: "ATTENTION",
              resource_type: "AUTOMATION",
              resource_id: "mandate-schwab",
              occurred_at: "2026-09-02T14:35:05Z",
              count: 1,
            },
          ],
        }}
      />,
    );

    expect(screen.getByText("Schwab market data needs review")).toBeVisible();
    expect(screen.getByText(/stopped before the model/i)).toBeVisible();
    expect(
      screen.getByRole("link", { name: /Review Schwab readiness/i }),
    ).toHaveAttribute("href", "/connections#schwab-market-readiness");
  });
});
