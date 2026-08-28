import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const navigation = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: navigation.refresh }),
}));

import { ShadowEvidenceReviewControls } from "./shadow-evidence-review-controls";

const fingerprint = "a".repeat(64);
const reviewableScorecard = {
  evidence_review_fingerprint: fingerprint,
  current_evidence_reviewed: false,
  evidence_gate: { status: "EVIDENCE_REVIEWABLE" },
};
const review = {
  id: "review-1",
  mandate_version: 4,
  evidence_fingerprint: fingerprint,
  gate_status: "EVIDENCE_REVIEWABLE" as const,
  one_hour_sample_size: 24,
  twenty_four_hour_sample_size: 20,
  evidence_window_hours: 171,
  schedule_healthy: true,
  last_schedule_status: "SUCCEEDED" as const,
  consecutive_schedule_failures: 0,
  execution_boundary: "SHADOW_ONLY" as const,
  live_execution_available: false as const,
  review_scope: "NON_LIVE_EVIDENCE_ONLY" as const,
  mfa_method: "totp" as const,
  reviewed_at: "2026-08-28T04:00:00Z",
};

describe("ShadowEvidenceReviewControls", () => {
  afterEach(() => {
    cleanup();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("shows no review form while durable evidence is still collecting", () => {
    render(
      <ShadowEvidenceReviewControls
        strategyInstanceId="instance-1"
        scorecard={{
          evidence_review_fingerprint: fingerprint,
          current_evidence_reviewed: false,
          evidence_gate: { status: "COLLECTING_EVIDENCE" },
        }}
      />,
    );

    expect(
      screen.getByText(/evidence is still collecting/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", {
        name: /record non-live evidence review/i,
      }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText(/live execution remain unavailable/i),
    ).toBeInTheDocument();
  });

  it("records the exact reviewable fingerprint with explicit confirmation and MFA", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(
      <ShadowEvidenceReviewControls
        strategyInstanceId="instance-1"
        scorecard={reviewableScorecard}
      />,
    );

    fireEvent.change(screen.getByLabelText(/authenticator code/i), {
      target: { value: "123456" },
    });
    fireEvent.click(
      screen.getByLabelText(/does not approve or enable live trading/i),
    );
    fireEvent.click(
      screen.getByRole("button", { name: /record non-live evidence review/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/strategy-instances/instance-1/shadow-evidence-reviews",
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          evidence_fingerprint: fingerprint,
          confirm_non_live_review: true,
          mfa_code: "123456",
        }),
      },
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      /no trading authority/i,
    );
    expect(navigation.refresh).toHaveBeenCalledTimes(1);
  });

  it("shows an immutable current review without another form", () => {
    render(
      <ShadowEvidenceReviewControls
        strategyInstanceId="instance-1"
        scorecard={{
          ...reviewableScorecard,
          current_evidence_reviewed: true,
          latest_evidence_review: {
            evidence_fingerprint: fingerprint,
            reviewed_at: "2026-08-28T04:00:00Z",
          },
        }}
      />,
    );

    expect(screen.getByText(/current snapshot reviewed/i)).toBeInTheDocument();
    expect(
      screen.getByText(/fri, 28 aug 2026 04:00:00 gmt/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByLabelText(/authenticator code/i),
    ).not.toBeInTheDocument();
  });

  it("keeps an older review visible but requires a fresh review for changed evidence", () => {
    render(
      <ShadowEvidenceReviewControls
        strategyInstanceId="instance-1"
        scorecard={{
          ...reviewableScorecard,
          latest_evidence_review: {
            evidence_fingerprint: "b".repeat(64),
            reviewed_at: "2026-08-28T03:00:00Z",
          },
        }}
      />,
    );

    expect(
      screen.getByText(/evidence changed after the prior review/i),
    ).toBeInTheDocument();
    expect(screen.getByLabelText(/authenticator code/i)).toBeInTheDocument();
  });

  it("renders current and preserved review evidence without implying authority", () => {
    render(
      <ShadowEvidenceReviewControls
        strategyInstanceId="instance-1"
        scorecard={{
          ...reviewableScorecard,
          current_evidence_reviewed: true,
          latest_evidence_review: review,
        }}
        initialReviews={[
          review,
          {
            ...review,
            id: "review-older",
            evidence_fingerprint: "b".repeat(64),
            reviewed_at: "2026-08-27T04:00:00Z",
          },
        ]}
      />,
    );

    expect(
      screen.getByText(/current evidence fingerprint/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/earlier evidence fingerprint/i),
    ).toBeInTheDocument();
    expect(screen.getAllByText(/grants no authority/i)).toHaveLength(2);
    expect(screen.getAllByText("24")).toHaveLength(2);
    expect(screen.getAllByText("SHADOW_ONLY")).toHaveLength(2);
  });

  it("loads earlier immutable review pages without duplicating records", async () => {
    const older = {
      ...review,
      id: "review-older",
      evidence_fingerprint: "b".repeat(64),
      reviewed_at: "2026-08-27T04:00:00Z",
    };
    const fetchMock = vi.fn(async () => ({
      ok: true,
      json: async () => ({
        evidence_reviews: [review, older],
        next_cursor: "",
      }),
    }));
    vi.stubGlobal("fetch", fetchMock);
    render(
      <ShadowEvidenceReviewControls
        strategyInstanceId="instance-1"
        scorecard={reviewableScorecard}
        initialReviews={[review]}
        initialCursor="older-cursor"
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: /load earlier reviews/i }),
    );
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/strategy-instances/instance-1/shadow-evidence-reviews?limit=8&cursor=older-cursor",
      { cache: "no-store" },
    );
    expect(screen.getByText("b".repeat(64))).toBeInTheDocument();
    expect(screen.getAllByText(fingerprint)).toHaveLength(1);
  });

  it("does not infer an empty ledger when history is unavailable", () => {
    render(
      <ShadowEvidenceReviewControls
        strategyInstanceId="instance-1"
        scorecard={reviewableScorecard}
        historyAvailable={false}
      />,
    );

    expect(
      screen.getByText(/history is temporarily unavailable/i),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(/no MFA-backed reviews recorded/i),
    ).not.toBeInTheDocument();
  });
});
