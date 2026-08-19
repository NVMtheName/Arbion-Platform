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

import { StrategyAutonomyControls } from "./strategy-autonomy-controls";

const base = {
  automationId: "mandate-1",
  currentVersion: 4,
  automationType: "STRATEGY",
  autonomyLevel: "RESEARCH_ONLY",
  executionMode: "PAPER",
  hasActiveInstance: false,
};

describe("StrategyAutonomyControls", () => {
  afterEach(() => {
    cleanup();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("creates only a confirmed non-live draft version", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true });
    vi.stubGlobal("fetch", fetchMock);
    render(<StrategyAutonomyControls {...base} />);

    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(
      screen.getByRole("button", { name: /create strategy-autonomous draft/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/automations/mandate-1/autonomy",
      {
        method: "PATCH",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          expected_version: 4,
          autonomy_level: "STRATEGY_AUTONOMOUS",
        }),
      },
    );
    expect(
      await screen.findByText(/new DRAFT version in PAPER mode/i),
    ).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent(
      /scheduling remains off/i,
    );
  });

  it("requires the existing capital claim to be released", () => {
    render(<StrategyAutonomyControls {...base} hasActiveInstance />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(
      screen.getByText(/finish the active or paused/i),
    ).toBeInTheDocument();
  });

  it("is unavailable for live or already-autonomous mandates", () => {
    const { rerender } = render(
      <StrategyAutonomyControls {...base} executionMode="LIVE" />,
    );
    expect(screen.queryByRole("region")).not.toBeInTheDocument();
    rerender(
      <StrategyAutonomyControls
        {...base}
        autonomyLevel="STRATEGY_AUTONOMOUS"
      />,
    );
    expect(screen.queryByRole("region")).not.toBeInTheDocument();
  });
});
