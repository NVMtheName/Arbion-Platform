import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const navigation = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: navigation.refresh }),
}));

import { CapitalBucketForm } from "./capital-bucket-form";

describe("CapitalBucketForm", () => {
  afterEach(() => {
    cleanup();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("creates an explicit non-executing allocation for an active account", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(
      <CapitalBucketForm
        accounts={[
          { id: "account-1", display_name: "Schwab account", status: "active" },
        ]}
      />,
    );

    fireEvent.change(screen.getByLabelText(/bucket name/i), {
      target: { value: "Paper allocation" },
    });
    fireEvent.change(screen.getByLabelText(/allocation \(usd\)/i), {
      target: { value: "1000" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: /create capital bucket/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith("/api/capital-buckets", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        financial_account_id: "account-1",
        name: "Paper allocation",
        allocation_type: "FIXED_AMOUNT",
        allocation_value: "1000",
        currency: "USD",
        protected_amount: "0",
        is_reserve: false,
      }),
    });
    expect(navigation.refresh).toHaveBeenCalledTimes(1);
    expect(screen.getByText(/not permission to trade/i)).toBeInTheDocument();
  });

  it("requires an active financial account", () => {
    render(
      <CapitalBucketForm
        accounts={[
          { id: "account-1", display_name: "Schwab account", status: "closed" },
        ]}
      />,
    );
    expect(screen.getByText(/connect and sync an active/i)).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /create capital bucket/i }),
    ).not.toBeInTheDocument();
  });

  it("creates compatible fixed budgets under an explicit shared ceiling", async () => {
    const fetchMock = vi.fn(async (...args: Parameters<typeof fetch>) => {
      void args;
      return { ok: true };
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <CapitalBucketForm
        accounts={[
          { id: "account-1", display_name: "Coinbase", status: "active" },
        ]}
      />,
    );

    fireEvent.change(screen.getByLabelText(/bucket name/i), {
      target: { value: "BTC sleeve" },
    });
    fireEvent.change(screen.getByLabelText(/allocation \(usd\)/i), {
      target: { value: "250" },
    });
    fireEvent.change(screen.getByLabelText(/shared account ceiling/i), {
      target: { value: "1000" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: /create capital bucket/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(
      JSON.parse(String(fetchMock.mock.calls[0]?.[1]?.body)),
    ).toMatchObject({
      financial_account_id: "account-1",
      name: "BTC sleeve",
      allocation_type: "FIXED_AMOUNT",
      allocation_value: "250",
      allocation_limit: "1000",
      is_reserve: false,
    });
  });
});
