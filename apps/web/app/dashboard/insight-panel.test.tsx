import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { InsightPanel } from "./insight-panel";

describe("InsightPanel", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("explains the boundary when no model is configured", () => {
    render(<InsightPanel configured={false} />);
    expect(
      screen.getByText(/has no access to your accounts/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /open connection hub/i }),
    ).toHaveAttribute("href", "/connections#ai-providers");
  });

  it("submits only the typed prompt and renders structured output", async () => {
    const fetchMock = vi.fn(async () => ({
      ok: true,
      json: async () => ({
        insight: {
          summary: "Diversification can reduce concentration risk.",
          key_points: ["Spread exposure."],
          risk_flags: ["Loss remains possible."],
          limitations: ["No live data was supplied."],
          requires_current_data: false,
          metadata: { model: "gpt-5.6-terra", profile: "core" },
        },
      }),
    }));
    vi.stubGlobal("fetch", fetchMock);
    render(<InsightPanel configured model="gpt-5.6-luna" />);

    fireEvent.click(screen.getByRole("radio", { name: /core/i }));

    fireEvent.change(
      screen.getByLabelText(/what would you like to understand/i),
      { target: { value: "Explain diversification" } },
    );
    fireEvent.click(screen.getByRole("button", { name: /generate insight/i }));

    expect(
      await screen.findByText(/diversification can reduce concentration risk/i),
    ).toBeInTheDocument();
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith("/api/neural/insight", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        prompt: "Explain diversification",
        profile: "core",
      }),
    });
    expect(screen.getByText("Spread exposure.")).toBeInTheDocument();
    expect(screen.getByText("Loss remains possible.")).toBeInTheDocument();
    expect(
      screen.getByText(/generated with gpt-5.6-terra · core profile/i),
    ).toBeInTheDocument();
  });

  it("defaults to the cheapest bounded route", () => {
    render(<InsightPanel configured model="gpt-5.6-luna" />);

    expect(screen.getByRole("radio", { name: /fast/i })).toBeChecked();
    expect(
      screen.getByText(/gpt-5.6 luna · 1 of 12 hourly credits/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/gpt-5.6 sol · 6 of 12 hourly credits/i),
    ).toBeInTheDocument();
  });
});
