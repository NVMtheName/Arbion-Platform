import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AutomationsHeader } from "./automations-header";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("AutomationsHeader", () => {
  it("keeps the shared heading target and a concise working-surface introduction", () => {
    render(<AutomationsHeader />);
    expect(
      screen.getByRole("heading", { level: 1, name: "AI Operations" }),
    ).toHaveAttribute("id", "automations-page-title");
    expect(
      screen.getByText(/Follow your engines, review saved decisions/),
    ).toBeInTheDocument();
    expect(screen.queryByText(/Every engine. One command surface/)).toBeNull();
  });

  it("links to existing setup and evidence without running or fetching anything", () => {
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);
    render(<AutomationsHeader />);
    expect(
      screen.getByRole("link", { name: "Set up AI engine" }),
    ).toHaveAttribute("href", "/automations/new");
    expect(
      screen.getByRole("link", { name: "Capital budgets" }),
    ).toHaveAttribute("href", "/capital");
    expect(
      screen.getByRole("link", { name: "Decision journal" }),
    ).toHaveAttribute("href", "/activity");
    expect(screen.queryByRole("button")).toBeNull();
    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
