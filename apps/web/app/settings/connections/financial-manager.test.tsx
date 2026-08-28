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

    expect(
      screen.getByText("How to create the right Coinbase key"),
    ).toBeVisible();
    expect(screen.getByLabelText("Coinbase key name")).toBeVisible();
    expect(screen.getByLabelText("ECDSA private key")).toBeVisible();
    expect(
      screen.getByText(/short, one-line secret is the wrong key type/),
    ).toBeVisible();
    expect(screen.getByText(/ECDSA \(ES256\)/)).toBeInTheDocument();
    expect(screen.getByText(/View permission: on/)).toBeInTheDocument();
    expect(screen.getByText(/Trade permission: optional/)).toBeInTheDocument();
    expect(screen.getByText(/Transfer permission: off/)).toBeInTheDocument();
    expect(screen.getByText(/52\.21\.127\.30/)).toBeInTheDocument();
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
            runtime_protected: false,
            protected_mandate_count: 0,
            active_strategy_count: 0,
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

  it("turns an expired Schwab connection into a clear weekly reconnect action", () => {
    render(
      <FinancialManager
        provider={{
          id: "schwab",
          label: "Charles Schwab",
          auth_type: "oauth2_authorization_code",
        }}
        entitled
        connections={[
          {
            id: "connection-1",
            provider: "schwab",
            display_name: "Charles Schwab",
            status: "expired",
            runtime_protected: false,
            protected_mandate_count: 0,
            active_strategy_count: 0,
          },
        ]}
        accounts={[
          {
            id: "account-1",
            provider_connection_id: "connection-1",
            provider: "schwab",
            display_name: "Schwab MARGIN ••••9555",
            status: "active",
          },
        ]}
      />,
    );

    expect(screen.getByText("Schwab reconnect required")).toBeVisible();
    expect(
      screen.getByText(/approve API access again each week/),
    ).toBeVisible();
    expect(
      screen.getByRole("button", { name: "Reconnect Schwab" }),
    ).toBeEnabled();
    expect(
      screen.getByRole("button", { name: "Refresh accounts" }),
    ).toBeDisabled();
  });

  it("shows the next Schwab renewal deadline while access is active", () => {
    render(
      <FinancialManager
        provider={{
          id: "schwab",
          label: "Charles Schwab",
          auth_type: "oauth2_authorization_code",
        }}
        entitled
        connections={[
          {
            id: "connection-1",
            provider: "schwab",
            display_name: "Charles Schwab",
            status: "active",
            runtime_protected: false,
            protected_mandate_count: 0,
            active_strategy_count: 0,
            authorization_expires_at: "2026-08-31T17:45:00Z",
          },
        ]}
        accounts={[]}
      />,
    );

    expect(
      screen.getByText("Weekly Schwab authorization active"),
    ).toBeVisible();
    expect(screen.getByText(/You can renew early/)).toBeVisible();
    expect(
      screen.getByRole("button", { name: "Renew Schwab access" }),
    ).toBeEnabled();
  });

  it("shows strategy continuity and blocks only destructive connection actions", () => {
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
            display_name: "Coinbase Production",
            status: "active",
            runtime_protected: true,
            protected_mandate_count: 1,
            active_strategy_count: 1,
          },
        ]}
        accounts={[]}
      />,
    );

    const continuity = screen.getByRole("region", {
      name: "Coinbase Production connection continuity",
    });
    expect(continuity).toHaveTextContent("Strategy continuity protected");
    expect(continuity).toHaveTextContent("1 active or paused engine");
    expect(continuity).toHaveTextContent("1 ready or paused mandate");
    expect(
      screen.getByRole("button", { name: "Refresh accounts" }),
    ).toBeEnabled();

    fireEvent.click(screen.getByText("Manage connection"));
    expect(screen.getByRole("button", { name: "Disable" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Disconnect" })).toBeDisabled();
  });

  it("extracts downloaded Coinbase JSON, sends it only to the connection endpoint, and clears it", async () => {
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

    fireEvent.change(screen.getByLabelText("Downloaded Coinbase key JSON"), {
      target: {
        value: JSON.stringify({
          name: "organizations/org/apiKeys/key",
          privateKey:
            "-----BEGIN EC PRIVATE KEY-----\nprivate-key-material\n-----END EC PRIVATE KEY-----",
        }),
      },
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
          private_key:
            "-----BEGIN EC PRIVATE KEY-----\nprivate-key-material\n-----END EC PRIVATE KEY-----",
        }),
      },
    );
    expect(screen.getByLabelText("Downloaded Coinbase key JSON")).toHaveValue(
      "",
    );
    expect(screen.getByRole("alert")).toHaveTextContent(
      "The provider did not accept these credentials.",
    );
  });

  it("rejects incomplete Coinbase JSON without sending credential material", () => {
    const fetchMock = vi.fn();
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

    fireEvent.change(screen.getByLabelText("Downloaded Coinbase key JSON"), {
      target: { value: '{"name":"organizations/org/apiKeys/key"}' },
    });
    fireEvent.click(screen.getByRole("button", { name: "Connect Coinbase" }));

    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent(
      /does not contain Coinbase’s name and privateKey values/,
    );
  });

  it("identifies a one-line Coinbase secret visibly at the submit action before sending it", async () => {
    const fetchMock = vi.fn();
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

    fireEvent.change(screen.getByLabelText("Coinbase key name"), {
      target: { value: "organizations/org/apiKeys/key" },
    });
    fireEvent.change(screen.getByLabelText("ECDSA private key"), {
      target: { value: "one-line-api-secret" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Connect Coinbase" }));

    expect(fetchMock).not.toHaveBeenCalled();
    const alert = screen.getByRole("alert");
    expect(alert).toHaveTextContent("Connection not completed.");
    expect(alert).toHaveTextContent(
      /one-line API secret, not the ECDSA private key/,
    );
    expect(alert.nextElementSibling).toBe(
      screen.getByRole("button", { name: "Connect Coinbase" }),
    );
    await waitFor(() => expect(alert).toHaveFocus());
  });
});
