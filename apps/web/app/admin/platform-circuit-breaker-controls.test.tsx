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

import { PlatformCircuitBreakerControls } from "./platform-circuit-breaker-controls";

describe("PlatformCircuitBreakerControls", () => {
  afterEach(() => {
    cleanup();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("engages the platform stop quickly without requesting broker or MFA action", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(<PlatformCircuitBreakerControls breaker={null} />);

    fireEvent.change(
      screen.getByLabelText(/why are you stopping platform actions/i),
      { target: { value: "Provider integrity requires immediate review" } },
    );
    fireEvent.click(screen.getByLabelText(/entire arbion platform/i));
    fireEvent.click(
      screen.getByRole("button", { name: /stop platform actions/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/admin/risk/circuit-breaker/engage",
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          reason: "Provider integrity requires immediate review",
          confirm: true,
        }),
      },
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      /deny every new risk-gated action/i,
    );
  });

  it("requires a fresh six-digit authenticator code to release", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(
      <PlatformCircuitBreakerControls
        breaker={{
          id: "global-breaker",
          scope: "GLOBAL",
          state: "OPEN",
          reason: "Provider integrity requires immediate review",
          source: "ADMIN_UI",
          engaged_at: "2026-08-27T19:00:00Z",
        }}
      />,
    );

    expect(screen.getByRole("alert")).toHaveTextContent(
      /every user, account, and automation/i,
    );
    fireEvent.change(
      screen.getByLabelText(/why is the platform safe to release/i),
      { target: { value: "All provider and platform evidence was reviewed" } },
    );
    fireEvent.change(screen.getByLabelText(/fresh authenticator code/i), {
      target: { value: "12a3456" },
    });
    fireEvent.click(screen.getByLabelText(/reviewed the incident/i));
    fireEvent.click(
      screen.getByRole("button", { name: /release platform stop/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/admin/risk/circuit-breaker/release",
      {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          reason: "All provider and platform evidence was reviewed",
          confirm: true,
          mfa_code: "123456",
        }),
      },
    );
    expect(screen.getByRole("status")).toHaveTextContent(/fresh mfa/i);
  });

  it("fails closed when the release code is rejected", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: false,
        json: async () => ({ error: { code: "INVALID_MFA_CODE" } }),
      })),
    );
    render(
      <PlatformCircuitBreakerControls
        breaker={{
          id: "global-breaker",
          scope: "GLOBAL",
          state: "OPEN",
          reason: "Platform review",
          source: "ADMIN_UI",
          engaged_at: "2026-08-27T19:00:00Z",
        }}
      />,
    );
    fireEvent.change(
      screen.getByLabelText(/why is the platform safe to release/i),
      { target: { value: "All platform evidence was reviewed" } },
    );
    fireEvent.change(screen.getByLabelText(/fresh authenticator code/i), {
      target: { value: "123456" },
    });
    fireEvent.click(screen.getByLabelText(/reviewed the incident/i));
    fireEvent.click(
      screen.getByRole("button", { name: /release platform stop/i }),
    );

    expect(await screen.findByRole("status")).toHaveTextContent(
      /fresh six-digit authenticator code/i,
    );
    expect(navigation.refresh).not.toHaveBeenCalled();
  });
});
