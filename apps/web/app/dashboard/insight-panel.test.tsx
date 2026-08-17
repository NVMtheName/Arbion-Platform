import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { InsightPanel } from "./insight-panel";

describe("InsightPanel", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("explains the boundary when no model is configured", () => {
    render(<InsightPanel configured={false} />);
    expect(
      screen.getByText(/has no access to your accounts/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /configure neural engine/i }),
    ).toHaveAttribute("href", "/settings/connections");
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
          metadata: { model: "gpt-5.6-luna" },
        },
      }),
    }));
    vi.stubGlobal("fetch", fetchMock);
    render(<InsightPanel configured model="gpt-5.6-luna" />);

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
      body: JSON.stringify({ prompt: "Explain diversification" }),
    });
    expect(screen.getByText("Spread exposure.")).toBeInTheDocument();
    expect(screen.getByText("Loss remains possible.")).toBeInTheDocument();
  });
});
