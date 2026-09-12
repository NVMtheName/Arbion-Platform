import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const navigation = vi.hoisted(() => ({
  refresh: vi.fn(),
  redirect: vi.fn((path: string) => {
    throw new Error(`redirect:${path}`);
  }),
}));
vi.mock("next/headers", () => ({
  cookies: async () => ({ toString: () => "session=test-only" }),
}));
vi.mock("next/navigation", () => ({
  redirect: navigation.redirect,
  useRouter: () => ({ refresh: navigation.refresh }),
}));
vi.mock("../../app-page-header", () => ({
  AppPageHeader: ({ contentHeadingId }: { contentHeadingId: string }) => (
    <header data-testid="shared-header" data-heading={contentHeadingId}>
      Shared navigation
    </header>
  ),
}));

import RiskSafetyPage from "./page";

const saved = {
  id: "example-stop",
  scope: "USER",
  state: "OPEN",
  source: "UI",
  reason: "Illustrative review of account conditions before further action",
  engaged_at: "2026-09-12T12:00:00Z",
};
function mockInventory(status = 200, circuit_breaker: unknown = null) {
  const fetch = vi.fn(async () => ({
    status,
    ok: status === 200,
    json: async () => ({ circuit_breaker }),
  }));
  vi.stubGlobal("fetch", fetch);
  vi.stubEnv("API_BASE_URL", "http://example.invalid");
  return fetch;
}

describe("Risk settings workspace", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
    vi.unstubAllEnvs();
    vi.clearAllMocks();
  });

  it.each([null, { ...saved, state: "CLOSED" }, saved])(
    "preserves the owner-scoped no-store loader and exact saved state %j",
    async (breaker) => {
      const fetch = mockInventory(200, breaker);
      render(await RiskSafetyPage());
      expect(fetch).toHaveBeenCalledExactlyOnceWith(
        "http://example.invalid/api/risk/circuit-breaker",
        { headers: { cookie: "session=test-only" }, cache: "no-store" },
      );
      expect(
        screen.getByRole("heading", { name: "Risk & safety", level: 1 }),
      ).toHaveAttribute("id", "risk-settings-title");
      expect(screen.getByTestId("shared-header")).toHaveAttribute(
        "data-heading",
        "risk-settings-title",
      );
      const links = within(
        screen.getByRole("navigation", { name: "Safety sections" }),
      ).getAllByRole("link");
      expect(links.map((link) => link.getAttribute("href"))).toEqual([
        "#owner-wide-stop",
        "#account-mandate-controls",
      ]);
      for (const link of links)
        expect(
          document.querySelector(link.getAttribute("href")!),
        ).toBeInTheDocument();
      expect(
        document
          .getElementById("owner-wide-stop")!
          .compareDocumentPosition(
            document.getElementById("account-mandate-controls")!,
          ),
      ).toBe(Node.DOCUMENT_POSITION_FOLLOWING);
      expect(
        screen.getByText(
          /These controls do not close positions or grant trading authority/,
        ),
      ).toBeInTheDocument();
      expect(navigation.refresh).not.toHaveBeenCalled();
      if (breaker?.state === "OPEN") {
        expect(screen.getByText("STOP ACTIVE")).toBeInTheDocument();
        expect(screen.getByText(saved.reason)).toBeInTheDocument();
        expect(document.querySelector("time")).toHaveAttribute(
          "dateTime",
          saved.engaged_at,
        );
        expect(document.querySelector("time")).toHaveTextContent(
          "Sat, 12 Sep 2026 12:00:00 GMT",
        );
        expect(screen.getByRole("alert")).toHaveTextContent(
          /does not close positions, revoke credentials, or send broker instructions/,
        );
      } else {
        expect(screen.getByText("STOP NOT ENGAGED")).toBeInTheDocument();
        expect(
          screen.getByRole("button", { name: "Stop All Arbion Actions" }),
        ).toHaveClass("danger");
      }
    },
  );

  it("keeps unavailable evidence distinct and presents no stale engage or release form", async () => {
    const fetch = mockInventory(503);
    render(await RiskSafetyPage());
    expect(screen.getByRole("status")).toHaveTextContent(
      /will not present a potentially stale engage or release action/,
    );
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    expect(screen.queryByText("STOP NOT ENGAGED")).not.toBeInTheDocument();
    for (const link of screen.getByRole("navigation").querySelectorAll("a"))
      expect(
        document.querySelector(link.getAttribute("href")!),
      ).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("preserves the authentication redirect", async () => {
    mockInventory(401);
    await expect(RiskSafetyPage()).rejects.toThrow("redirect:/login");
    expect(navigation.redirect).toHaveBeenCalledExactlyOnceWith("/login");
  });

  it("preserves transport failure instead of treating it as no saved stop", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new Error("Illustrative transport unavailable");
      }),
    );
    await expect(RiskSafetyPage()).rejects.toThrow(
      "Illustrative transport unavailable",
    );
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});
