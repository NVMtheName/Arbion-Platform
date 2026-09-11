import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ProductPreview } from "./product-preview";

describe("Illustrative product preview", () => {
  afterEach(cleanup);

  it("clearly labels sample values and never fetches financial data", () => {
    const fetch = vi.spyOn(globalThis, "fetch");
    render(<ProductPreview />);
    expect(
      screen.getByText("Example values · not account data"),
    ).toBeInTheDocument();
    expect(screen.getByText("No live prices shown")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: "AI Engine" }));
    expect(
      screen.getByRole("tabpanel", { name: "AI Engine" }),
    ).not.toHaveAttribute("hidden");
    fireEvent.click(screen.getByRole("tab", { name: "Decision Journal" }));
    expect(
      screen.getByRole("tabpanel", { name: "Decision Journal" }),
    ).not.toHaveAttribute("hidden");
    expect(fetch).not.toHaveBeenCalled();
    fetch.mockRestore();
  });

  it("supports arrow, Home, and End keys with one tab stop and stable panels", () => {
    render(<ProductPreview />);
    const tabs = screen.getAllByRole("tab");
    tabs[0].focus();
    fireEvent.keyDown(tabs[0], { key: "ArrowRight" });
    expect(tabs[1]).toHaveFocus();
    expect(tabs[1]).toHaveAttribute("aria-selected", "true");
    expect(tabs[0]).toHaveAttribute("tabindex", "-1");
    fireEvent.keyDown(tabs[1], { key: "End" });
    expect(tabs[2]).toHaveFocus();
    fireEvent.keyDown(tabs[2], { key: "ArrowRight" });
    expect(tabs[0]).toHaveFocus();
    fireEvent.keyDown(tabs[0], { key: "ArrowLeft" });
    expect(tabs[2]).toHaveFocus();
    fireEvent.keyDown(tabs[2], { key: "Home" });
    expect(tabs[0]).toHaveFocus();
    expect(screen.getAllByRole("tabpanel", { hidden: true })).toHaveLength(3);
    expect(screen.getAllByRole("tabpanel")).toHaveLength(1);
    tabs.forEach((tab, index) =>
      expect(tab).toHaveAttribute("aria-controls", `preview-panel-${index}`),
    );
  });
});
