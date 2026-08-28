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
});
