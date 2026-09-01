import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { PaperPortfolio } from "./paper-portfolio-summary";

const navigation = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: navigation.refresh }),
}));

import { PaperEvidenceReviewControls } from "./paper-evidence-review-controls";

const fingerprint = "a".repeat(64);
const portfolio = {
  strategy_instance_id: "instance-1",
  evidence_review_fingerprint: fingerprint,
  current_evidence_reviewed: false,
  evidence_readiness: { status: "EVIDENCE_REVIEWABLE" },
} as PaperPortfolio;

describe("PaperEvidenceReviewControls", () => {
  afterEach(() => {
    cleanup();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("does not expose a review form before the complete gate is reviewable", () => {
    render(
      <PaperEvidenceReviewControls
        strategyInstanceId="instance-1"
        portfolio={{
          ...portfolio,
          evidence_readiness: {
            ...portfolio.evidence_readiness!,
            status: "COLLECTING_EVIDENCE",
          },
        }}
      />,
    );

    expect(
      screen.getByText(/Paper evidence is still collecting/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /record Paper evidence review/i }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText(/No mandate, strategy, account/i),
    ).toBeInTheDocument();
  });

  it("records the exact gate fingerprint with explicit confirmation and MFA", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(
      <PaperEvidenceReviewControls
        strategyInstanceId="instance-1"
        portfolio={portfolio}
      />,
    );

    fireEvent.change(screen.getByLabelText(/authenticator code/i), {
      target: { value: "123456" },
    });
    fireEvent.click(
      screen.getByLabelText(/does not approve or enable live trading/i),
    );
    fireEvent.click(
      screen.getByRole("button", { name: /record Paper evidence review/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/strategy-instances/instance-1/paper-evidence-reviews",
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          evidence_fingerprint: fingerprint,
          confirm_paper_review: true,
          mfa_code: "123456",
        }),
      },
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      /No promotion, trading authority/i,
    );
    expect(navigation.refresh).toHaveBeenCalledTimes(1);
  });

  it("shows an immutable current checkpoint without another form", () => {
    render(
      <PaperEvidenceReviewControls
        strategyInstanceId="instance-1"
        portfolio={{
          ...portfolio,
          current_evidence_reviewed: true,
          latest_evidence_review: {
            id: "review-1",
            strategy_instance_id: "instance-1",
            financial_account_id: "account-1",
            mandate_id: "mandate-1",
            mandate_version: 1,
            evidence_fingerprint: fingerprint,
            gate_status: "EVIDENCE_REVIEWABLE",
            evidence_started_at: "2026-08-29T12:00:00Z",
            evidence_eligible_at: "2026-09-05T12:00:00Z",
            evidence_as_of: "2026-09-05T12:00:00Z",
            evidence_window_hours: 168,
            decision_count: 20,
            portfolio_version: 1,
            portfolio_updated_at: "2026-09-05T12:00:00Z",
            latest_checkpoint_run_id: "run-1",
            latest_checkpoint_as_of: "2026-09-05T12:00:00Z",
            scheduler_sample_count: 20,
            scheduler_success_count: 20,
            scheduler_failure_count: 0,
            last_schedule_status: "SUCCEEDED",
            consecutive_schedule_failures: 0,
            route_continuity_status: "STABLE",
            input_coverage_status: "COMPLETE",
            input_freshness_status: "CURRENT_AT_DECISION",
            ledger_contract_status: "RECONCILED",
            no_live_safety_status: "CLEAR",
            execution_boundary: "PAPER_SIMULATION_ONLY",
            review_scope: "PAPER_NON_LIVE_EVIDENCE_ONLY",
            grants_authority: false,
            live_promotion_available: false,
            mfa_method: "totp",
            reviewed_at: "2026-09-05T12:05:00Z",
            created_at: "2026-09-05T12:05:00Z",
          },
        }}
      />,
    );

    expect(
      screen.getByText(/Current Paper checkpoint reviewed/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByLabelText(/authenticator code/i),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText(/Sat, 05 Sep 2026 12:05:00 GMT/i),
    ).toBeInTheDocument();
  });
});
