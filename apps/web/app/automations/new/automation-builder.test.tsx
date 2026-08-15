import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import AutomationBuilder from "./automation-builder";

describe("AutomationBuilder prerequisites", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("blocks drafts until a financial account is connected", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request) => ({
        json: async () =>
          String(input).includes("capital-buckets")
            ? { capital_buckets: null }
            : String(input).includes("connections/ai")
              ? { connections: null }
              : { accounts: null },
      })),
    );

    render(<AutomationBuilder />);

    expect(
      await screen.findByText(/connect and sync a financial account/i),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: /save draft/i }),
      ).toBeDisabled(),
    );
    expect(
      screen.getByRole("link", { name: /manage connections/i }),
    ).toHaveAttribute("href", "/settings/connections");
  });
});
