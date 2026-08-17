import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import AutomationBuilder from "./automation-builder";

describe("AutomationBuilder prerequisites", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

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

  it("does not attach protected reserve buckets to automation drafts", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request) => ({
        json: async () =>
          String(input).includes("capital-buckets")
            ? {
                capital_buckets: [
                  {
                    ID: "reserve-1",
                    Name: "Never touch",
                    Status: "ACTIVE",
                    IsReserve: true,
                    FinancialAccountID: "account-1",
                  },
                ],
              }
            : String(input).includes("connections/ai")
              ? { connections: [] }
              : {
                  accounts: [
                    {
                      id: "account-1",
                      display_name: "Schwab account",
                      status: "active",
                    },
                  ],
                },
      })),
    );

    render(<AutomationBuilder />);

    expect(
      await screen.findByText(/create a capital bucket/i),
    ).toBeInTheDocument();
    expect(screen.getByLabelText(/capital bucket/i)).toBeDisabled();
    expect(screen.getByRole("button", { name: /save draft/i })).toBeDisabled();
  });

  it("offers only buckets owned by the selected active account", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: string | URL | Request) => ({
        json: async () =>
          String(input).includes("capital-buckets")
            ? {
                capital_buckets: [
                  {
                    ID: "bucket-1",
                    Name: "First allocation",
                    Status: "ACTIVE",
                    IsReserve: false,
                    FinancialAccountID: "account-1",
                  },
                  {
                    ID: "bucket-2",
                    Name: "Second allocation",
                    Status: "ACTIVE",
                    IsReserve: false,
                    FinancialAccountID: "account-2",
                  },
                ],
              }
            : String(input).includes("connections/ai")
              ? { connections: [] }
              : {
                  accounts: [
                    {
                      id: "account-1",
                      display_name: "First account",
                      status: "active",
                    },
                    {
                      id: "account-2",
                      display_name: "Second account",
                      status: "active",
                    },
                  ],
                },
      })),
    );

    render(<AutomationBuilder />);

    expect(
      await screen.findByRole("option", { name: "First allocation" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("option", { name: "Second allocation" }),
    ).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText(/^account$/i), {
      target: { value: "account-2" },
    });

    expect(
      screen.getByRole("option", { name: "Second allocation" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("option", { name: "First allocation" }),
    ).not.toBeInTheDocument();
  });
});
