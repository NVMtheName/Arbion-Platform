import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import type { AnchorHTMLAttributes } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("next/link", () => ({
  default: ({
    children,
    onClick,
    ...props
  }: AnchorHTMLAttributes<HTMLAnchorElement>) => (
    <a
      {...props}
      onClick={(event) => {
        onClick?.(event);
        event.preventDefault();
      }}
    >
      {children}
    </a>
  ),
}));
vi.mock("next/navigation", () => ({
  usePathname: () => "/accounts",
}));

import {
  AppNavigation,
  projectConnectionNavigationHealth,
  resolveNavigationScrollLeft,
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
    window.sessionStorage.clear();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("keeps deliberate horizontal scroll when the active tab is visible", () => {
    expect(
      resolveNavigationScrollLeft({
        retainedScrollLeft: 180,
        scrollWidth: 960,
        viewportWidth: 320,
        activeStart: 220,
        activeWidth: 80,
      }),
    ).toBe(180);
  });

  it("reveals an active tab beyond either edge without vertical scrolling", () => {
    expect(
      resolveNavigationScrollLeft({
        retainedScrollLeft: 100,
        scrollWidth: 960,
        viewportWidth: 320,
        activeStart: 850,
        activeWidth: 80,
      }),
    ).toBe(610);
    expect(
      resolveNavigationScrollLeft({
        retainedScrollLeft: 500,
        scrollWidth: 960,
        viewportWidth: 320,
        activeStart: 40,
        activeWidth: 80,
      }),
    ).toBe(40);
  });

  it("clamps saved overflow and fails closed on malformed geometry", () => {
    expect(
      resolveNavigationScrollLeft({
        retainedScrollLeft: 900,
        scrollWidth: 960,
        viewportWidth: 320,
        activeStart: 700,
        activeWidth: 80,
      }),
    ).toBe(640);
    expect(
      resolveNavigationScrollLeft({
        retainedScrollLeft: Number.NaN,
        scrollWidth: 960,
        viewportWidth: 320,
        activeStart: 700,
        activeWidth: 80,
      }),
    ).toBe(0);
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

  it("shows immediate route-switching feedback without moving the navigation", () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ connections: [connection()] }),
      }),
    );
    render(<AppNavigation />);

    const navigation = screen.getByRole("navigation", {
      name: "Application navigation",
    });
    fireEvent.click(screen.getByRole("link", { name: "Markets" }));

    expect(navigation).toHaveAttribute("aria-busy", "true");
    expect(navigation).toHaveClass("is-switching");
    expect(screen.getByRole("link", { name: "Markets" })).toHaveClass(
      "is-pending",
    );
    expect(
      screen.getByRole("status", { name: "Opening Markets" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Portfolio" })).toHaveAttribute(
      "aria-current",
      "page",
    );
  });

  it("keeps the current tab visible when the navigation viewport resizes", () => {
    let resizeCallback: ResizeObserverCallback | undefined;
    const disconnect = vi.fn();
    class NavigationResizeObserver {
      constructor(callback: ResizeObserverCallback) {
        resizeCallback = callback;
      }
      observe() {}
      disconnect() {
        disconnect();
      }
    }
    vi.stubGlobal("ResizeObserver", NavigationResizeObserver);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ connections: [connection()] }),
      }),
    );
    const { unmount } = render(<AppNavigation />);
    const navigation = screen.getByRole("navigation", {
      name: "Application navigation",
    });
    const active = screen.getByRole("link", { name: "Portfolio" });
    Object.defineProperties(navigation, {
      clientWidth: { configurable: true, value: 320 },
      scrollWidth: { configurable: true, value: 960 },
    });
    Object.defineProperties(active, {
      offsetLeft: { configurable: true, value: 700 },
      offsetWidth: { configurable: true, value: 80 },
    });

    act(() => resizeCallback?.([], {} as ResizeObserver));

    expect(navigation.scrollLeft).toBe(460);
    expect(window.sessionStorage.getItem("arbion-navigation-scroll-left")).toBe(
      "460",
    );
    unmount();
    expect(disconnect).toHaveBeenCalledOnce();
  });

  it("does not show switching feedback for the current route or a new-tab click", () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ connections: [connection()] }),
      }),
    );
    render(<AppNavigation />);

    const navigation = screen.getByRole("navigation", {
      name: "Application navigation",
    });
    fireEvent.click(screen.getByRole("link", { name: "Portfolio" }));
    expect(navigation).not.toHaveAttribute("aria-busy");

    fireEvent.click(screen.getByRole("link", { name: "Markets" }), {
      metaKey: true,
    });
    expect(navigation).not.toHaveAttribute("aria-busy");
  });

  it("remains usable when browser session storage is unavailable", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new DOMException("Storage unavailable");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("Storage unavailable");
    });
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ connections: [connection()] }),
      }),
    );

    expect(() => render(<AppNavigation />)).not.toThrow();
    expect(
      screen.getByRole("navigation", { name: "Application navigation" }),
    ).toBeInTheDocument();
  });
});
