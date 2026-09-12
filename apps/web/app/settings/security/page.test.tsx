import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const redirect = vi.hoisted(() =>
  vi.fn((path: string) => {
    throw new Error(`redirect:${path}`);
  }),
);
vi.mock("next/headers", () => ({
  cookies: async () => ({ toString: () => "session=test-only" }),
}));
vi.mock("next/navigation", () => ({
  redirect,
  useRouter: () => ({ refresh: vi.fn(), replace: vi.fn() }),
}));
vi.mock("../../app-page-header", () => ({
  AppPageHeader: () => <header>Shared navigation</header>,
}));
import SecurityPage from "./page";

const sessionInventory = {
  active_count: 3,
  other_count: 2,
  current: {
    created_at: "2026-09-12T10:00:00Z",
    expires_at: "2026-09-12T12:00:00Z",
  },
};
function inventoryFetch({
  failedIndex = -1,
  status = 200,
  sessions = sessionInventory,
  emailVerified = true,
}: {
  failedIndex?: number;
  status?: number;
  sessions?: unknown;
  emailVerified?: boolean;
} = {}) {
  const fetch = vi.fn();
  [
    { user: { email: "owner@example.com", email_verified: emailVerified } },
    { mfa: { enabled: true, recovery_codes_remaining: 4 } },
    {
      activities: [
        {
          id: "example-event",
          action: "auth.login",
          occurred_at: "2026-09-12T10:00:00Z",
        },
      ],
      next_cursor: "earlier",
    },
    { session_inventory: sessions },
  ].forEach((body, index) =>
    fetch.mockResolvedValueOnce({
      status: index === failedIndex ? status : 200,
      ok: index !== failedIndex || status === 200,
      json: async () => body,
    }),
  );
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

describe("Security workspace loading and navigation", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    vi.clearAllMocks();
  });

  it("preserves four owner-scoped no-store requests and links to every existing section", async () => {
    const fetch = inventoryFetch();
    render(await SecurityPage());
    expect(
      screen.getByRole("heading", { level: 1, name: "Security & access" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        /Signed in as owner@example.com. Email verification is complete/,
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/4 unused recovery codes remain/),
    ).toBeInTheDocument();
    expect(screen.getByText("Successful sign-in")).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledTimes(4);
    ["me", "mfa", "security-activity?limit=20", "sessions"].forEach(
      (path, index) => {
        expect(fetch).toHaveBeenNthCalledWith(
          index + 1,
          expect.stringContaining(`/api/auth/${path}`),
          {
            headers: { cookie: "session=test-only" },
            cache: "no-store",
          },
        );
      },
    );
    const links = screen
      .getByRole("navigation", { name: "Security sections" })
      .querySelectorAll("a");
    expect(links).toHaveLength(4);
    for (const link of links) {
      const id = link.getAttribute("href")!.slice(1);
      expect(document.getElementById(id)).not.toBeNull();
    }
  });

  it.each([0, 1, 2, 3])(
    "retains the authentication redirect for unauthorized response %i",
    async (failedIndex) => {
      inventoryFetch({ failedIndex, status: 401 });
      await expect(SecurityPage()).rejects.toThrow("redirect:/login");
      expect(redirect).toHaveBeenCalledTimes(1);
    },
  );

  it.each([0, 1])(
    "does not render controls if required security response %i is unavailable",
    async (failedIndex) => {
      inventoryFetch({ failedIndex, status: 503 });
      await expect(SecurityPage()).rejects.toThrow(
        "Unable to load account security",
      );
    },
  );

  it("keeps unavailable saved activity visibly distinct from an empty timeline", async () => {
    inventoryFetch({ failedIndex: 2, status: 503 });
    render(await SecurityPage());
    expect(screen.getByRole("status")).toHaveTextContent(
      "Security activity could not be verified.",
    );
    expect(screen.queryByText("Successful sign-in")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Load earlier activity" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Change Password" }),
    ).toBeEnabled();
  });

  it.each([
    { failedIndex: 3, status: 503 },
    { sessions: null },
    { sessions: { ...sessionInventory, other_count: 1 } },
    { sessions: { ...sessionInventory, active_count: 2.5 } },
    {
      sessions: {
        ...sessionInventory,
        current: { created_at: "invalid", expires_at: "2026-09-12T12:00:00Z" },
      },
    },
  ])(
    "does not infer verified session state from incomplete evidence %#",
    async (options) => {
      inventoryFetch(options);
      render(await SecurityPage());
      expect(screen.getByRole("status")).toHaveTextContent(
        "Active sessions could not be verified.",
      );
      expect(
        screen.queryByLabelText("Session summary"),
      ).not.toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: "Sign Out Other Sessions" }),
      ).toBeDisabled();
      expect(
        screen.getByRole("button", { name: "Sign Out All Sessions" }),
      ).toBeEnabled();
      expect(
        screen.getByRole("region", { name: "Active browser sessions" }),
      ).toHaveClass("is-review");
    },
  );

  it("retains the exact unverified-email explanation", async () => {
    inventoryFetch({ emailVerified: false });
    render(await SecurityPage());
    expect(
      screen.getByText(
        /Email verification is not yet enabled for private testing/,
      ),
    ).toBeInTheDocument();
  });
});
