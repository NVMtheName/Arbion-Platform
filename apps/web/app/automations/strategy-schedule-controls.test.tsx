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
  emailDeliveryAvailable: true,
  financialProvider: "schwab",
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
    fireEvent.click(screen.getByLabelText(/after each scheduled evaluation/i));
    fireEvent.click(screen.getByLabelText(/option needs lifecycle review/i));
    fireEvent.click(
      screen.getByLabelText(/first consecutive scheduler failure/i),
    );
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
        notifications: {
          evaluation_completed: true,
          lifecycle_required: true,
          first_failure: true,
        },
      },
    });
    expect(await screen.findByText(/new DRAFT version/i)).toBeInTheDocument();
  });

  it("keeps notification choices opt-in and informational", () => {
    render(<StrategyScheduleControls {...base} />);
    expect(
      screen.getByText(/go only to your verified Arbion address/i),
    ).toBeInTheDocument();
    for (const name of [
      /after each scheduled evaluation/i,
      /option needs lifecycle review/i,
      /first consecutive scheduler failure/i,
    ]) {
      expect(screen.getByLabelText(name)).not.toBeChecked();
    }
  });

  it("offers immutable drift-review alerts only for AI Shadow schedules", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <StrategyScheduleControls
        {...base}
        automationType="AI_AUTONOMOUS"
        autonomyLevel="FULL_AUTONOMOUS"
        executionMode="SHADOW"
        financialProvider="coinbase"
      />,
    );

    fireEvent.click(screen.getByLabelText(/enable guarded/i));
    fireEvent.click(
      screen.getByLabelText(/tradable-inventory change needs review/i),
    );
    fireEvent.click(screen.getByRole("button", { name: /save schedule/i }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(JSON.parse(String(fetchMock.mock.calls[0][1]?.body))).toEqual({
      expected_version: 4,
      schedule_conditions: {
        enabled: true,
        interval_minutes: 60,
        session: "CONTINUOUS",
        notifications: {
          evaluation_completed: false,
          lifecycle_required: false,
          first_failure: false,
          reconciliation_review_required: true,
        },
      },
    });
    expect(screen.getByText(/never acknowledge a change/i)).toBeInTheDocument();
  });

  it("saves an AI Paper cadence without offering broker drift review", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <StrategyScheduleControls
        {...base}
        automationType="AI_AUTONOMOUS"
        autonomyLevel="FULL_AUTONOMOUS"
        financialProvider="coinbase"
      />,
    );

    expect(
      screen.queryByLabelText(/tradable-inventory change needs review/i),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByLabelText(/enable guarded/i));
    fireEvent.click(screen.getByRole("button", { name: /save schedule/i }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(JSON.parse(String(fetchMock.mock.calls[0][1]?.body))).toEqual({
      expected_version: 4,
      schedule_conditions: {
        enabled: true,
        interval_minutes: 60,
        session: "CONTINUOUS",
        notifications: {
          evaluation_completed: false,
          lifecycle_required: false,
          first_failure: false,
        },
      },
    });
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

  it("disables email choices when delivery is unavailable", () => {
    render(
      <StrategyScheduleControls {...base} emailDeliveryAvailable={false} />,
    );
    fireEvent.click(screen.getByLabelText(/enable guarded/i));
    expect(
      screen.getByText(/email delivery is not configured/i),
    ).toBeInTheDocument();
    expect(
      screen.getByLabelText(/after each scheduled evaluation/i),
    ).toBeDisabled();
  });

  it.each([
    ["MARKET_DATA_STALE", /after the data refreshes/i],
    ["MARKET_DATA_DELAYED", /explicitly marked.*delayed/i],
    ["MARKET_DATA_REALTIME_UNCONFIRMED", /rather than guessing/i],
    ["MARKET_DATA_NOT_REALTIME", /stopped before the AI model/i],
    ["NO_ELIGIBLE_OPTION_CONTRACTS", /review those filters/i],
    ["STRATEGY_CONFIGURATION_CHANGED", /review the automation/i],
    ["WAITING_FOR_LIFECYCLE", /record the explicit paper option lifecycle/i],
    ["OUTSIDE_SESSION", /no action needed/i],
    ["SESSION_CALENDAR_UNAVAILABLE", /operator must extend it/i],
    ["AI_PROVIDER_RATE_LIMITED", /retry at the next scheduled cycle/i],
    ["AI_REQUEST_INVALID", /strict schema contract/i],
    ["INTERNAL", /operator review/i],
  ])("explains the safe scheduled result %s", (code, guidance) => {
    render(
      <StrategyScheduleControls
        {...base}
        conditions={{ enabled: true }}
        instanceId="instance-1"
        schedulerEnabled
        runtime={{
          enabled: true,
          last_status: "FAILED",
          last_error_code: code,
          consecutive_failures: 1,
        }}
      />,
    );

    expect(screen.getByText(guidance)).toBeInTheDocument();
    expect(screen.getByText(/no broker order was sent/i)).toBeInTheDocument();
  });

  it("uses the selected financial provider in safe recovery guidance", () => {
    render(
      <StrategyScheduleControls
        {...base}
        automationType="AI_AUTONOMOUS"
        autonomyLevel="FULL_AUTONOMOUS"
        financialProvider="coinbase"
        conditions={{ enabled: true }}
        instanceId="instance-1"
        schedulerEnabled
        runtime={{
          enabled: true,
          last_status: "FAILED",
          last_error_code: "PROVIDER",
          consecutive_failures: 1,
        }}
      />,
    );
    expect(
      screen.getByText(/Coinbase read data was unavailable/i),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Schwab market data/i)).not.toBeInTheDocument();
  });
});
