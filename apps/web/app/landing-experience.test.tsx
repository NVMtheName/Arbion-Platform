import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { LandingExperience } from "./landing-experience";

describe("Arbion landing experience", () => {
  afterEach(cleanup);

  it("presents a branded, truthful path into the product", () => {
    render(<LandingExperience />);

    expect(screen.getAllByRole("img", { name: "Arbion" })).toHaveLength(2);
    expect(
      screen.getByRole("heading", { name: /see your money as a system/i }),
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
