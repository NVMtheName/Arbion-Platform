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

import { AccountCircuitBreakerControls } from "./account-circuit-breaker-controls";

describe("AccountCircuitBreakerControls", () => {
  afterEach(() => {
    cleanup();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("engages an owner-confirmed account stop", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(
      <AccountCircuitBreakerControls
        accountId="account-1"
        accountName="Schwab brokerage"
        breaker={null}
      />,
    );

    fireEvent.change(
      screen.getByLabelText(/why are you stopping this account/i),
      {
        target: { value: "Connection state needs review" },
      },
    );
    fireEvent.click(screen.getByLabelText(/this account only/i));
    fireEvent.click(
      screen.getByRole("button", { name: /engage account stop/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/accounts/account-1/circuit-breaker/engage",
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          reason: "Connection state needs review",
          confirm: true,
        }),
      },
    );
    expect(screen.getByRole("status")).toHaveTextContent(/deny new actions/i);
    expect(navigation.refresh).toHaveBeenCalledTimes(1);
  });

  it("shows durable active state and releases only after review", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(
      <AccountCircuitBreakerControls
        accountId="account-1"
        accountName="Schwab brokerage"
        breaker={{
          id: "breaker-1",
          scope: "ACCOUNT",
          scope_id: "account-1",
          state: "OPEN",
          reason: "Connection state needs review",
          source: "UI",
          engaged_at: "2026-08-27T14:00:00Z",
        }}
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent(
      /deny every new action/i,
    );
    fireEvent.change(screen.getByLabelText(/why is it safe to release/i), {
      target: { value: "Connection and holdings were reviewed" },
    });
    fireEvent.click(screen.getByLabelText(/reviewed the cause/i));
    fireEvent.click(
      screen.getByRole("button", { name: /release account stop/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/accounts/account-1/circuit-breaker/release",
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          reason: "Connection and holdings were reviewed",
          confirm: true,
        }),
      },
    );
    expect(screen.getByRole("status")).toHaveTextContent(/may evaluate/i);
  });

  it("does not refresh when the account-stop state conflicts", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: false,
        json: async () => ({ error: { code: "CIRCUIT_BREAKER_CONFLICT" } }),
      })),
    );
    render(
      <AccountCircuitBreakerControls
        accountId="account-1"
        accountName="Coinbase"
        breaker={null}
      />,
    );
    fireEvent.change(
      screen.getByLabelText(/why are you stopping this account/i),
      {
        target: { value: "Unexpected account state needs review" },
      },
    );
    fireEvent.click(screen.getByLabelText(/this account only/i));
    fireEvent.click(
      screen.getByRole("button", { name: /engage account stop/i }),
    );

    expect(await screen.findByRole("status")).toHaveTextContent(
      /state changed/i,
    );
    expect(navigation.refresh).not.toHaveBeenCalled();
  });
});
