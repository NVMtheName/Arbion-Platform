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

import { StrategyEvaluationControls } from "./strategy-evaluation-controls";

const base = {
  automationId: "mandate-1",
  currentVersion: 2,
  status: "READY",
  executionMode: "PAPER",
  strategyIdentifier: "wheel",
  instanceId: "",
  instanceStatus: "",
  strategyParameters: {},
};

const configured = {
  symbols: ["AAPL"],
  minimum_dte: 20,
  maximum_dte: 60,
  target_delta: "0.30",
  target_delta_min: "0.20",
  target_delta_max: "0.40",
  maximum_contracts: 1,
  assignment_handling_policy: "continue_wheel",
};

describe("StrategyEvaluationControls", () => {
  afterEach(() => {
    cleanup();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("saves normalized parameters as a draft without evaluating", async () => {
    const fetchMock = vi
      .fn<
        (
          input: RequestInfo | URL,
          init?: RequestInit,
        ) => Promise<{ ok: boolean }>
      >()
      .mockResolvedValue({ ok: true });
    vi.stubGlobal("fetch", fetchMock);
    render(<StrategyEvaluationControls {...base} />);

    fireEvent.change(screen.getByLabelText(/symbols/i), {
      target: { value: " aapl " },
    });
    fireEvent.click(
      screen.getByRole("button", { name: /save parameters.*draft/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    const request = fetchMock.mock.calls[0];
    expect(request[0]).toBe("/api/automations/mandate-1/strategy-parameters");
    expect(JSON.parse(String(request[1]?.body))).toMatchObject({
      expected_version: 2,
      strategy_parameters: { symbols: ["AAPL"], maximum_contracts: 1 },
    });
    expect(
      await screen.findByText(/no evaluation or broker order/i),
    ).toBeInTheDocument();
  });

  it("runs only an explicit non-live evaluation", async () => {
    const fetchMock = vi
      .fn<
        (
          input: RequestInfo | URL,
          init?: RequestInit,
        ) => Promise<{ ok: boolean; json: () => Promise<object> }>
      >()
      .mockResolvedValue({
        ok: true,
        json: async () => ({
          evaluation: {
            risk_decision: "DENY",
            risk_reason_codes: ["CAPITAL_LIMIT_EXCEEDED"],
            execution: { status: "RISK_DENIED" },
          },
        }),
      });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <StrategyEvaluationControls
        {...base}
        instanceId="instance-1"
        instanceStatus="ACTIVE"
        strategyParameters={configured}
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: /run manual paper evaluation/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/strategy-instances/instance-1/evaluate",
    );
    const body = JSON.parse(String(fetchMock.mock.calls[0][1]?.body));
    expect(body.event_id).toMatch(/^manual:/);
    expect(
      await screen.findByText(/no broker order was sent/i),
    ).toBeInTheDocument();
  });

  it("does not offer manual evaluation while the instance is paused", () => {
    render(
      <StrategyEvaluationControls
        {...base}
        instanceId="instance-1"
        instanceStatus="PAUSED"
        strategyParameters={configured}
      />,
    );
    expect(
      screen.queryByRole("button", { name: /run manual paper evaluation/i }),
    ).not.toBeInTheDocument();
    expect(screen.getByText(/strategy is paused/i)).toBeInTheDocument();
  });
});
