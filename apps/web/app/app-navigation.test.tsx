import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("next/navigation", () => ({
  usePathname: () => "/accounts",
}));

import {
  AppNavigation,
  projectConnectionNavigationHealth,
} from "./app-navigation";

const observedAt = "2026-09-01T16:00:00Z";

function connection(overrides: Record<string, unknown> = {}) {
  return {
    id: "coinbase-connection",
    provider: "coinbase",
    status: "active",
    last_synced_at: "2026-09-01T15:55:00Z",
    ...overrides,
  };
}

describe("persistent connection navigation health", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("shows a current state from exact saved financial connection evidence", () => {
    const health = projectConnectionNavigationHealth(
      [
        connection(),
        connection({
          id: "schwab-connection",
          provider: "schwab",
          authorization_expires_at: "2026-09-08T16:00:00Z",
        }),
      ],
      observedAt,
    );

    expect(health).toEqual({
      state: "CURRENT",
      label: "2 financial connections are current",
      connectionCount: 2,
      attentionCount: 0,
    });
  });

  it("uses the exact 24-hour authorization boundary for renewal attention", () => {
    const health = projectConnectionNavigationHealth(
      [connection({ authorization_expires_at: "2026-09-02T16:00:00Z" })],
      observedAt,
    );
    const overdue = projectConnectionNavigationHealth(
      [connection({ authorization_expires_at: "2026-09-01T15:59:59Z" })],
      observedAt,
    );

    expect(health.state).toBe("RENEWAL_NEEDED");
    expect(overdue.state).toBe("RENEWAL_NEEDED");
    expect(health.attentionCount).toBe(1);
  });

  it("puts a failed saved connection ahead of a separate renewal warning", () => {
    const health = projectConnectionNavigationHealth(
      [
        connection({ status: "expired" }),
        connection({
          id: "schwab-connection",
          provider: "schwab",
          authorization_expires_at: "2026-09-02T10:00:00Z",
        }),
      ],
      observedAt,
    );

    expect(health.state).toBe("CONNECTION_FAILED");
    expect(health.attentionCount).toBe(2);
  });

  it("fails closed on duplicate, malformed, or future connection evidence", () => {
    const duplicate = connection();
    expect(
      projectConnectionNavigationHealth(
        [duplicate, { ...duplicate }],
        observedAt,
      ).state,
    ).toBe("UNAVAILABLE");
    expect(
      projectConnectionNavigationHealth(
        [connection({ last_synced_at: "2026-09-01T16:00:01Z" })],
        observedAt,
      ).state,
    ).toBe("UNAVAILABLE");
    expect(projectConnectionNavigationHealth({}, observedAt).state).toBe(
      "UNAVAILABLE",
    );
  });

  it("distinguishes an exact empty inventory from unavailable evidence", () => {
    expect(projectConnectionNavigationHealth([], observedAt)).toEqual({
      state: "NOT_CONNECTED",
      label: "No financial account connected",
      connectionCount: 0,
      attentionCount: 0,
    });
  });

  it("reserves the navigation signal while loading and updates it read-only", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ connections: [connection()] }),
    });
    vi.stubGlobal("fetch", fetchMock);
    render(<AppNavigation />);

    const connectionsLink = screen.getByRole("link", { name: "Connections" });
    const initialStatus = screen.getByRole("status", {
      name: "Checking financial connection health",
    });
    expect(connectionsLink).toHaveClass("has-health");
    expect(initialStatus).toHaveClass("connection-navigation-health");

    await waitFor(() =>
      expect(
        screen.getByRole("status", {
          name: "1 financial connection is current",
        }),
      ).toHaveClass("is-current"),
    );
    expect(fetchMock).toHaveBeenCalledWith("/api/connections/financial", {
      cache: "no-store",
      signal: expect.any(AbortSignal),
    });
  });
});
