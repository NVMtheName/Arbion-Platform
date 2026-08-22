import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { FinancialManager } from "./financial-manager";

describe("FinancialManager", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("shows the founder-safe Coinbase key requirements", () => {
    render(
      <FinancialManager
        provider={{
          id: "coinbase",
          label: "Coinbase",
          auth_type: "jwt_key_pair",
        }}
        entitled
        connections={[]}
        accounts={[]}
      />,
    );

    expect(screen.getByText(/ECDSA \(ES256\)/)).toBeVisible();
    expect(screen.getByText(/View permission: on/)).toBeVisible();
    expect(screen.getByText(/Trade permission: optional/)).toBeVisible();
    expect(screen.getByText(/Transfer permission: off/)).toBeVisible();
    expect(screen.getByText(/52\.21\.127\.30/)).toBeVisible();
    expect(
      screen.getByRole("button", { name: "Connect Coinbase" }),
    ).toBeEnabled();
  });

  it("distinguishes a Coinbase trading grant from Arbion execution authority", () => {
    render(
      <FinancialManager
        provider={{
          id: "coinbase",
          label: "Coinbase",
          auth_type: "jwt_key_pair",
        }}
        entitled
        connections={[
          {
            id: "connection-1",
            provider: "coinbase",
            display_name: "Coinbase",
            status: "active",
          },
        ]}
        accounts={[
          {
            id: "account-1",
            provider_connection_id: "connection-1",
            provider: "coinbase",
            display_name: "Coinbase Portfolio",
            status: "active",
            capabilities: {
              provider_trade_authorization: "SUPPORTED",
            },
          },
        ]}
      />,
    );

    expect(screen.getByText("Coinbase Trade permission granted")).toBeVisible();
    expect(screen.getByText(/order submission remains locked/)).toBeVisible();
  });

  it("sends credentials only to the Coinbase connection endpoint and clears the private key", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: false,
      json: async () => ({
        error: { message: "The provider did not accept these credentials." },
      }),
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <FinancialManager
        provider={{
          id: "coinbase",
          label: "Coinbase",
          auth_type: "jwt_key_pair",
        }}
        entitled
        connections={[]}
        accounts={[]}
      />,
    );

    fireEvent.change(screen.getByLabelText("API key name"), {
      target: { value: "organizations/org/apiKeys/key" },
    });
    fireEvent.change(screen.getByLabelText("ECDSA private key"), {
      target: { value: "private-key-material" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Connect Coinbase" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/connections/financial/coinbase",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          key_name: "organizations/org/apiKeys/key",
          private_key: "private-key-material",
        }),
      },
    );
    expect(screen.getByLabelText("ECDSA private key")).toHaveValue("");
    expect(screen.getByRole("alert")).toHaveTextContent(
      "The provider did not accept these credentials.",
    );
  });
});
