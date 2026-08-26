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

import { AIShadowEvaluationControls } from "./ai-shadow-evaluation-controls";

describe("AIShadowEvaluationControls", () => {
  afterEach(() => {
    cleanup();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("explains the immutable one-hour repeat-action guard", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        evaluation: {
          ai_decision: "PROPOSE",
          confidence: "MEDIUM",
          risk_decision: "DENY",
          risk_reason_codes: ["REPEAT_ACTION_COOLDOWN_ACTIVE"],
          execution: { status: "RISK_DENIED" },
        },
      }),
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <AIShadowEvaluationControls
        status="READY"
        instanceId="instance-1"
        instanceStatus="ACTIVE"
        parameters={{
          objective: "Preserve capital.",
          max_proposal_notional: "1",
        }}
        symbols={["BTC", "ETH", "XRP"]}
      />,
    );

    expect(
      screen.getByText("One hour · same symbol and side"),
    ).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "Run AI Shadow cycle now" }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(
      await screen.findByText(/held it at the deterministic control gate/i),
    ).toBeInTheDocument();
    expect(navigation.refresh).toHaveBeenCalledTimes(1);
  });
});
