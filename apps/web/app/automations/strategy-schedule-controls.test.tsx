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

import { StrategyScheduleControls } from "./strategy-schedule-controls";

const base = {
  automationId: "mandate-1",
  currentVersion: 4,
  automationType: "STRATEGY",
  autonomyLevel: "STRATEGY_AUTONOMOUS",
  executionMode: "PAPER",
  instanceId: "",
  schedulerEnabled: false,
  conditions: {},
};

describe("StrategyScheduleControls", () => {
  afterEach(() => {
    cleanup();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("saves an opt-in non-live cadence as a new draft", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true });
    vi.stubGlobal("fetch", fetchMock);
    render(<StrategyScheduleControls {...base} />);

    fireEvent.click(screen.getByLabelText(/enable guarded/i));
    fireEvent.change(screen.getByLabelText(/evaluation interval/i), {
      target: { value: "120" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save schedule/i }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/automations/mandate-1/schedule",
    );
    expect(JSON.parse(String(fetchMock.mock.calls[0][1]?.body))).toEqual({
      expected_version: 4,
      schedule_conditions: {
        enabled: true,
        interval_minutes: 120,
        session: "US_EQUITIES_REGULAR",
      },
    });
    expect(await screen.findByText(/new DRAFT version/i)).toBeInTheDocument();
  });

  it("keeps scheduling unavailable outside the guarded mandate boundary", () => {
    render(
      <StrategyScheduleControls
        {...base}
        autonomyLevel="CONFIRM_EACH"
        executionMode="LIVE"
      />,
    );
    expect(
      screen.getByRole("button", { name: /save schedule/i }),
    ).toBeDisabled();
    expect(
      screen.getByText(/requires STRATEGY_AUTONOMOUS/i),
    ).toBeInTheDocument();
  });
});
