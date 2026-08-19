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

import { AutomationCircuitBreakerControls } from "./automation-circuit-breaker-controls";

describe("AutomationCircuitBreakerControls", () => {
  afterEach(() => {
    cleanup();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("engages an owner-confirmed emergency stop with a reason", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(
      <AutomationCircuitBreakerControls
        automationId="mandate-1"
        breaker={null}
      />,
    );

    fireEvent.change(screen.getByLabelText(/why are you stopping it/i), {
      target: { value: "Unexpected market data needs review" },
    });
    fireEvent.click(screen.getByLabelText(/immediately blocks new actions/i));
    fireEvent.click(
      screen.getByRole("button", { name: /engage emergency stop/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/automations/mandate-1/circuit-breaker/engage",
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          reason: "Unexpected market data needs review",
          confirm: true,
        }),
      },
    );
    expect(screen.getByRole("status")).toHaveTextContent(/deny new actions/i);
    expect(navigation.refresh).toHaveBeenCalledTimes(1);
  });

  it("shows durable active state and requires a release explanation", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(
      <AutomationCircuitBreakerControls
        automationId="mandate-1"
        breaker={{
          id: "breaker-1",
          scope: "AUTOMATION",
          scope_id: "mandate-1",
          state: "OPEN",
          reason: "Unexpected market data needs review",
          source: "UI",
          engaged_at: "2026-08-19T18:00:00Z",
        }}
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent(/deny new actions/i);
    expect(
      screen.getByText("Unexpected market data needs review"),
    ).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText(/why is it safe to release/i), {
      target: { value: "Market data and settings were reviewed" },
    });
    fireEvent.click(screen.getByLabelText(/reviewed the cause/i));
    fireEvent.click(
      screen.getByRole("button", { name: /release emergency stop/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/automations/mandate-1/circuit-breaker/release",
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          reason: "Market data and settings were reviewed",
          confirm: true,
        }),
      },
    );
    expect(screen.getByRole("status")).toHaveTextContent(/may evaluate/i);
  });

  it("never carries an engage confirmation into the release form", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    const { rerender } = render(
      <AutomationCircuitBreakerControls
        automationId="mandate-1"
        breaker={null}
      />,
    );
    fireEvent.change(screen.getByLabelText(/why are you stopping it/i), {
      target: { value: "Unexpected market data needs review" },
    });
    fireEvent.click(screen.getByLabelText(/immediately blocks new actions/i));
    fireEvent.click(
      screen.getByRole("button", { name: /engage emergency stop/i }),
    );
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));

    rerender(
      <AutomationCircuitBreakerControls
        automationId="mandate-1"
        breaker={{
          id: "breaker-1",
          scope: "AUTOMATION",
          scope_id: "mandate-1",
          state: "OPEN",
          reason: "Unexpected market data needs review",
          source: "UI",
          engaged_at: "2026-08-19T18:00:00Z",
        }}
      />,
    );

    expect(screen.getByLabelText(/reviewed the cause/i)).not.toBeChecked();
  });

  it("does not refresh when the breaker state conflicts", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: false,
        json: async () => ({
          error: { code: "CIRCUIT_BREAKER_CONFLICT" },
        }),
      })),
    );
    render(
      <AutomationCircuitBreakerControls
        automationId="mandate-1"
        breaker={null}
      />,
    );
    fireEvent.change(screen.getByLabelText(/why are you stopping it/i), {
      target: { value: "Unexpected market data needs review" },
    });
    fireEvent.click(screen.getByLabelText(/immediately blocks new actions/i));
    fireEvent.click(
      screen.getByRole("button", { name: /engage emergency stop/i }),
    );

    expect(await screen.findByRole("status")).toHaveTextContent(
      /state changed/i,
    );
    expect(navigation.refresh).not.toHaveBeenCalled();
  });
});
