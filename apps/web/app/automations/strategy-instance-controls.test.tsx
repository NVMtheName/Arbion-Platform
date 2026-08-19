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

import { StrategyInstanceControls } from "./strategy-instance-controls";

describe("StrategyInstanceControls", () => {
  afterEach(() => {
    cleanup();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("finishes only after explicit non-live confirmation", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(
      <StrategyInstanceControls
        instanceId="instance-1"
        status="ACTIVE"
        stateVersion={7}
      />,
    );

    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(
      screen.getByRole("button", { name: /finish non-live strategy/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/strategy-instances/instance-1/finish",
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          expected_state_version: 7,
          confirm_non_live_finish: true,
        }),
      },
    );
    expect(screen.getByText(/no schwab order/i)).toBeInTheDocument();
    expect(navigation.refresh).toHaveBeenCalledTimes(1);
  });

  it("hides the irreversible action for a completed simulation", () => {
    render(
      <StrategyInstanceControls
        instanceId="instance-1"
        status="COMPLETED"
        stateVersion={8}
      />,
    );
    expect(
      screen.queryByRole("button", { name: /finish non-live strategy/i }),
    ).not.toBeInTheDocument();
  });

  it("keeps the claim when PAPER still has simulated exposure", async () => {
    const fetchMock = vi.fn(async () => ({
      ok: false,
      json: async () => ({ error: { code: "PAPER_POSITION_OPEN" } }),
    }));
    vi.stubGlobal("fetch", fetchMock);
    render(
      <StrategyInstanceControls
        instanceId="instance-1"
        status="ACTIVE"
        stateVersion={2}
      />,
    );

    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(
      screen.getByRole("button", { name: /finish non-live strategy/i }),
    );

    expect(
      await screen.findByText(/still has an open option or share position/i),
    ).toBeInTheDocument();
    expect(navigation.refresh).not.toHaveBeenCalled();
  });
});
