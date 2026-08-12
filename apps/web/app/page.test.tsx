import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import Home from "./page";

describe("Home", () => {
  it("describes the platform foundation", () => {
    render(<Home />);
    expect(
      screen.getByRole("heading", { name: /disciplined decisions/i }),
    ).toBeInTheDocument();
    expect(screen.getByText(/foundation online/i)).toBeInTheDocument();
  });
});
