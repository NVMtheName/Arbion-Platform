import { cleanup, render, screen, within } from "@testing-library/react";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("next/navigation", () => ({ usePathname: () => "/connections" }));

import { AppPageHeader } from "./app-page-header";
import { AppRouteLoading } from "./app-route-loading";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("consistent application navigation", () => {
  it("removes the redundant Dashboard back link but retains the tab and brand", () => {
    render(<AppPageHeader showConnectionHealth={false} />);
    expect(
      screen.queryByRole("link", { name: "← Dashboard" }),
    ).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Dashboard" })).toHaveAttribute(
      "href",
      "/dashboard",
    );
    expect(screen.getByRole("link", { name: "Arbion home" })).toHaveAttribute(
      "href",
      "/dashboard",
    );
    expect(
      document.querySelector(".app-page-header-actions"),
    ).toBeEmptyDOMElement();
  });

  it("also suppresses an explicit Dashboard return without removing its navigation tab", () => {
    render(
      <AppPageHeader
        backHref="/dashboard"
        backLabel="Dashboard"
        showConnectionHealth={false}
      />,
    );
    expect(document.querySelector(".app-back-link")).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Dashboard" })).toHaveAttribute(
      "href",
      "/dashboard",
    );
  });

  it("preserves an explicit contextual return and custom account actions", () => {
    const view = render(
      <AppPageHeader
        backHref="/accounts"
        backLabel="Accounts"
        showConnectionHealth={false}
      />,
    );
    expect(screen.getByRole("link", { name: "← Accounts" })).toHaveAttribute(
      "href",
      "/accounts",
    );
    view.rerender(
      <AppPageHeader
        actions={<button>Log out</button>}
        showConnectionHealth={false}
      />,
    );
    expect(screen.getByRole("button", { name: "Log out" })).toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "← Accounts" }),
    ).not.toBeInTheDocument();
  });

  it("prefetches only a data-free shell with interruptible navigation", () => {
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    render(<AppRouteLoading section="Connections" />);
    expect(
      screen.getByRole("heading", { name: "Connections" }),
    ).toHaveAttribute("id", "route-loading-title");
    expect(screen.getByText("Loading your workspace…")).toHaveAttribute(
      "role",
      "status",
    );
    expect(
      within(screen.getByRole("navigation")).getAllByRole("link"),
    ).toHaveLength(8);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(fetch).not.toHaveBeenCalled();
    expect(document.querySelector(".app-route-skeleton")).toHaveAttribute(
      "aria-hidden",
      "true",
    );
    expect(document.querySelector("#app-main-content")).toHaveAttribute(
      "aria-labelledby",
      "route-loading-title",
    );
  });

  it.each([
    "dashboard",
    "accounts",
    "markets",
    "automations",
    "activity",
    "capital",
    "connections",
    "settings/connections",
    "settings/security",
    "settings/risk",
  ])("provides a lightweight streaming boundary for %s", (route) => {
    const source = readFileSync(
      resolve(process.cwd(), "app", route, "loading.tsx"),
      "utf8",
    );
    expect(source).toContain("AppRouteLoading");
    expect(source).not.toMatch(
      /fetch\(|cookies\(|useEffect|refresh\(|revalidate|force-cache/,
    );
  });

  it("uses shared neutral tokens and keeps meaningful state colors distinct", () => {
    const legacy = readFileSync(
      resolve(process.cwd(), "app/styles.css"),
      "utf8",
    );
    const theme = readFileSync(
      resolve(process.cwd(), "app/precision.css"),
      "utf8",
    );
    expect(legacy).not.toMatch(/#07110e|#091a14|#0b1b15cc|#244237|#315447/);
    for (const token of [
      ...(legacy + theme).matchAll(/var\(--(surface-[a-z-]+)\)/g),
    ]) {
      expect(theme).toContain(`--${token[1]}:`);
    }
    expect(legacy).toContain("--brand-mint: #5ee0a0");
    expect(legacy).toContain("#ca6969");
    expect(legacy).toContain("#e6af47");
    expect(theme).toContain("prefers-reduced-motion: no-preference");
    expect(theme).toContain("animation: app-content-arrive 160ms ease-out");
    expect(theme).toContain(".app-page-header-actions");
  });
});
