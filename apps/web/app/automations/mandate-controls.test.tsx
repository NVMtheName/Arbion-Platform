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

import { MandateControls } from "./mandate-controls";

const base = {
  automationId: "mandate-1",
  currentVersion: 3,
  automationType: "STRATEGY",
  executionMode: "PAPER",
  strategyIdentifier: "wheel",
  instanceExists: false,
};

describe("MandateControls", () => {
  afterEach(() => {
    cleanup();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("marks a draft ready without claiming execution", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(<MandateControls {...base} status="DRAFT" />);

    fireEvent.click(
      screen.getByRole("button", { name: /mark ready.*no execution/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith("/api/automations/mandate-1/ready", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ expected_version: 3 }),
    });
    expect(
      await screen.findByText(/no strategy was run and no order was sent/i),
    ).toBeInTheDocument();
  });

  it("initializes only a simulated paper portfolio", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(<MandateControls {...base} status="READY" />);

    fireEvent.change(screen.getByLabelText(/starting simulated cash/i), {
      target: { value: "2500" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: /initialize paper strategy/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/automations/mandate-1/strategy/initialize",
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ starting_cash: "2500" }),
      },
    );
    expect(
      await screen.findByText(/no broker order was sent/i),
    ).toBeInTheDocument();
  });

  it("initializes an AI Paper engine with isolated simulated cash", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(
      <MandateControls
        {...base}
        automationType="AI_AUTONOMOUS"
        strategyIdentifier="—"
        status="READY"
      />,
    );

    fireEvent.change(screen.getByLabelText(/starting simulated cash/i), {
      target: { value: "900" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: /initialize paper ai engine/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/automations/mandate-1/strategy/initialize",
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ starting_cash: "900" }),
      },
    );
    expect(
      await screen.findByText(/PAPER strategy initialized/i),
    ).toBeInTheDocument();
  });

  it("explains a protected capital bucket rejection", async () => {
    const fetchMock = vi.fn(async () => ({
      ok: false,
      json: async () => ({ error: { code: "PAPER_CAPITAL_LIMIT" } }),
    }));
    vi.stubGlobal("fetch", fetchMock);
    render(<MandateControls {...base} status="READY" />);

    fireEvent.change(screen.getByLabelText(/starting simulated cash/i), {
      target: { value: "2501" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: /initialize paper strategy/i }),
    );

    expect(
      await screen.findByText(/must fit within the selected capital bucket/i),
    ).toBeInTheDocument();
    expect(navigation.refresh).not.toHaveBeenCalled();
  });

  it("explains aggregate account-level capital isolation", async () => {
    const fetchMock = vi.fn(async () => ({
      ok: false,
      json: async () => ({ error: { code: "ACCOUNT_CAPITAL_IN_USE" } }),
    }));
    vi.stubGlobal("fetch", fetchMock);
    render(<MandateControls {...base} status="READY" />);

    fireEvent.change(screen.getByLabelText(/starting simulated cash/i), {
      target: { value: "1" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: /initialize paper strategy/i }),
    );

    expect(
      await screen.findByText(/overlaps an active or paused reservation/i),
    ).toBeInTheDocument();
    expect(navigation.refresh).not.toHaveBeenCalled();
  });

  it("explains when a bucket cannot establish an exact reservation", async () => {
    const fetchMock = vi.fn(async () => ({
      ok: false,
      json: async () => ({
        error: { code: "CAPITAL_RESERVATION_UNAVAILABLE" },
      }),
    }));
    vi.stubGlobal("fetch", fetchMock);
    render(<MandateControls {...base} status="READY" executionMode="SHADOW" />);

    fireEvent.click(
      screen.getByRole("button", { name: /initialize shadow strategy/i }),
    );

    expect(
      await screen.findByText(/cannot produce an exact non-live reservation/i),
    ).toBeInTheDocument();
    expect(navigation.refresh).not.toHaveBeenCalled();
  });

  it("does not offer initialization for live configuration", () => {
    render(<MandateControls {...base} status="READY" executionMode="LIVE" />);
    expect(
      screen.queryByRole("button", { name: /initialize/i }),
    ).not.toBeInTheDocument();
    expect(screen.getByText(/cannot place or prepare/i)).toBeInTheDocument();
  });

  it("holds initialization when the current readiness snapshot is blocked", () => {
    render(
      <MandateControls
        {...base}
        status="READY"
        initializationBlocked
        paperStartingCashLimit="250.0000000000"
      />,
    );

    expect(screen.getByLabelText(/starting simulated cash/i)).toBeDisabled();
    expect(
      screen.getByRole("button", { name: /initialize paper strategy/i }),
    ).toBeDisabled();
    expect(screen.getByText(/current exact maximum/i)).toHaveTextContent(
      "250.0000000000 USD",
    );
    expect(
      screen.getByText(/resolve the blocked strategy readiness/i),
    ).toBeInTheDocument();
  });
});
