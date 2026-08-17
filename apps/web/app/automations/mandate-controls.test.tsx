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

  it("does not offer initialization for live configuration", () => {
    render(<MandateControls {...base} status="READY" executionMode="LIVE" />);
    expect(
      screen.queryByRole("button", { name: /initialize/i }),
    ).not.toBeInTheDocument();
    expect(screen.getByText(/cannot place or prepare/i)).toBeInTheDocument();
  });
});
