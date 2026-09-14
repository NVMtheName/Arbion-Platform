import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";

import { LandingExperience } from "./landing-experience";
import Home, { dynamic } from "./page";

describe("Arbion landing experience", () => {
  afterEach(cleanup);

  it("keeps the public server-rendered page free of financial values and account reads", () => {
    const fetch = vi.spyOn(globalThis, "fetch").mockImplementation(() => {
      throw new Error("Public landing page attempted an account/network read");
    });
    try {
      const html = renderToStaticMarkup(<Home />);
      expect(dynamic).toBe("error");
      expect(html).toContain("Private by design");
      expect(html).not.toMatch(/[$€£]\s*[\d,.]|Schwab|Coinbase/i);
      expect(html).not.toMatch(
        /\/api\/(?:financial|accounts)|provider_account_id|access_token|refresh_token/i,
      );
      expect(fetch).not.toHaveBeenCalled();
    } finally {
      fetch.mockRestore();
    }
  });

  it("presents a branded, truthful path into the product", () => {
    render(<LandingExperience />);

    expect(screen.getAllByRole("img", { name: "Arbion" })).toHaveLength(2);
    expect(
      screen.getByRole("heading", {
        name: /your accounts.*your AI.*one command/i,
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /enter command center/i }),
    ).toHaveAttribute("href", "/login");
    expect(
      screen.getByLabelText(/illustrative arbion command-center preview/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/no live prices shown/i)).toBeInTheDocument();
    expect(screen.getByText(/no live execution path/i)).toBeInTheDocument();
    expect(
      screen.getByRole("heading", {
        name: /know the boundaries before you connect/i,
      }),
    ).toBeInTheDocument();
    expect(screen.getByText(/NVM Technologies, LLC/i)).toBeInTheDocument();
    expect(
      screen.getByText(/not a broker, exchange, bank/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/full legal suite is under counsel review/i),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /contact support@arbion.ai/i }),
    ).toHaveAttribute("href", "mailto:support@arbion.ai");
  });
});
