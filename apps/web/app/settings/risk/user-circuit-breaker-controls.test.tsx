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

import { UserCircuitBreakerControls } from "./user-circuit-breaker-controls";

describe("UserCircuitBreakerControls", () => {
  afterEach(() => {
    cleanup();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("engages the owner-wide stop only after explicit confirmation", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(<UserCircuitBreakerControls breaker={null} />);

    fireEvent.change(
      screen.getByLabelText(/why are you stopping all actions/i),
      {
        target: { value: "Provider state requires an owner review" },
      },
    );
    fireEvent.click(screen.getByLabelText(/every new arbion action/i));
    fireEvent.click(
      screen.getByRole("button", { name: /stop all arbion actions/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith("/api/risk/circuit-breaker/engage", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        reason: "Provider state requires an owner review",
        confirm: true,
      }),
    });
    expect(screen.getByRole("status")).toHaveTextContent(
      /deny every new action/i,
    );
    expect(navigation.refresh).toHaveBeenCalledTimes(1);
  });

  it("shows durable active state and requires a reviewed release", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(
      <UserCircuitBreakerControls
        breaker={{
          id: "breaker-1",
          scope: "USER",
          scope_id: "owner",
          state: "OPEN",
          reason: "Provider state requires an owner review",
          source: "UI",
          engaged_at: "2026-08-27T15:00:00Z",
        }}
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent(/deny new actions/i);
    fireEvent.change(screen.getByLabelText(/why is it safe to release/i), {
      target: { value: "All accounts and conditions were reviewed" },
    });
    fireEvent.click(screen.getByLabelText(/reviewed the cause/i));
    fireEvent.click(
      screen.getByRole("button", { name: /release owner-wide stop/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/risk/circuit-breaker/release",
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          reason: "All accounts and conditions were reviewed",
          confirm: true,
        }),
      },
    );
    expect(screen.getByRole("status")).toHaveTextContent(/may evaluate/i);
  });

  it("fails closed on conflicting owner-stop state", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: false,
        json: async () => ({ error: { code: "CIRCUIT_BREAKER_CONFLICT" } }),
      })),
    );
    render(<UserCircuitBreakerControls breaker={null} />);
    fireEvent.change(
      screen.getByLabelText(/why are you stopping all actions/i),
      {
        target: { value: "All accounts require a fresh review" },
      },
    );
    fireEvent.click(screen.getByLabelText(/every new arbion action/i));
    fireEvent.click(
      screen.getByRole("button", { name: /stop all arbion actions/i }),
    );

    expect(await screen.findByRole("status")).toHaveTextContent(
      /state changed/i,
    );
    expect(navigation.refresh).not.toHaveBeenCalled();
  });
});
