import type { ComponentProps } from "react";
import Link from "next/link";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("../../app-page-header", () => ({
  AppPageHeader: ({ contentHeadingId }: { contentHeadingId: string }) => (
    <header data-testid="shared-header" aria-labelledby={contentHeadingId}>
      Shared navigation
    </header>
  ),
}));

import { ConnectionsWorkspace } from "./connections-workspace";

const base: ComponentProps<typeof ConnectionsWorkspace> = {
  connections: [],
  providers: [
    {
      id: "openai",
      label: "OpenAI",
      credential_types: ["api_key"],
      capabilities: ["text"],
    },
  ],
  aiEntitled: true,
  preference: null,
  preferenceAvailable: true,
  financialProviders: [
    {
      id: "coinbase",
      label: "Coinbase",
      auth_type: "api_key",
      availability: "implemented",
      configured: true,
    },
    {
      id: "schwab",
      label: "Charles Schwab",
      auth_type: "oauth2",
      availability: "implemented",
      configured: true,
    },
    {
      id: "etrade",
      label: "E*TRADE",
      auth_type: "oauth1",
      availability: "planned",
      configured: false,
    },
  ],
  financialProvidersAvailable: true,
  financialEntitled: true,
  financialConnections: [],
  financialAccounts: [],
  inventoryAvailable: true,
  operatingEvidence: (
    <details open>
      <summary>Saved evidence needs review</summary>
      <Link href="/accounts/account-a#account-sync-history-title">
        Immutable account evidence
      </Link>
    </details>
  ),
};
const financialConnection = {
  id: "schwab-connection",
  provider: "schwab",
  display_name: "Brokerage connection",
  status: "active",
  runtime_protected: true,
  protected_mandate_count: 1,
  active_strategy_count: 1,
};

describe("ConnectionsWorkspace", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("puts actionable setup before advanced operating evidence and preserves the shared shell", () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    render(<ConnectionsWorkspace {...base} />);
    expect(
      screen.getByRole("heading", { level: 1, name: "Connections" }),
    ).toBeInTheDocument();
    expect(screen.getByTestId("shared-header")).toHaveAttribute(
      "aria-labelledby",
      "connections-page-title",
    );
    const account = screen.getByRole("region", {
      name: "Connect a financial account",
    });
    const ai = screen.getByRole("region", { name: "Connect an AI provider" });
    const health = screen.getByRole("region", {
      name: "Saved connection health",
    });
    expect(
      account.compareDocumentPosition(ai) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      ai.compareDocumentPosition(health) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(
      screen.getByText("Saved evidence needs review").closest("details"),
    ).toHaveAttribute("open");
    expect(
      screen.getByRole("link", { name: "Immutable account evidence" }),
    ).toHaveAttribute("href", "/accounts/account-a#account-sync-history-title");
    expect(
      screen.getByRole("link", { name: "Review connection health" }),
    ).toHaveAttribute("href", "#connection-health");
    expect(screen.getByLabelText("Coinbase key name")).toBeInTheDocument();
    expect(screen.getByLabelText("ECDSA private key")).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "Add API key for OpenAI" }),
    );
    expect(screen.getByLabelText("API key")).toHaveAttribute(
      "type",
      "password",
    );
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("keeps planned and unconfigured providers distinct from usable connections", () => {
    render(
      <ConnectionsWorkspace
        {...base}
        financialProviders={base.financialProviders.map((p) =>
          p.id === "schwab" ? { ...p, configured: false } : p,
        )}
      />,
    );
    expect(
      screen.getByText(/Planned — not yet available: E\*TRADE/),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Connect E\*TRADE/ }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("article", { name: "E*TRADE" }),
    ).not.toBeInTheDocument();
    expect(
      within(screen.getByRole("article", { name: "Charles Schwab" })).getByText(
        "This connection is temporarily unavailable.",
      ),
    ).toBeVisible();
    expect(
      screen.queryByRole("button", { name: "Connect Charles Schwab" }),
    ).not.toBeInTheDocument();
  });

  it("fails closed on unavailable inventory instead of offering duplicate account setup", () => {
    render(
      <ConnectionsWorkspace
        {...base}
        inventoryAvailable={false}
        financialConnections={[financialConnection]}
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent(
      "Some saved setup information is unavailable",
    );
    expect(
      screen.getByRole("link", {
        name: /1 Financial account Status unavailable/,
      }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Connect Coinbase" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Refresh accounts" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByText("Connected", { exact: true }),
    ).not.toBeInTheDocument();
    expect(screen.getAllByText("Unavailable", { exact: true })).toHaveLength(2);
    expect(
      screen.getByRole("button", { name: "Add API key for OpenAI" }),
    ).toBeEnabled();
  });

  it("preserves provider expiry, linked accounts and runtime removal protection", () => {
    render(
      <ConnectionsWorkspace
        {...base}
        financialConnections={[{ ...financialConnection, status: "expired" }]}
        financialAccounts={[
          {
            id: "account-a",
            provider_connection_id: financialConnection.id,
            provider: "schwab",
            display_name: "Linked brokerage",
            status: "active",
          },
        ]}
      />,
    );
    expect(
      screen.getByText(/A financial connection needs attention/),
    ).toBeVisible();
    expect(
      screen.getByRole("button", { name: "Reconnect Schwab" }),
    ).toBeEnabled();
    expect(screen.getByRole("button", { name: "Disconnect" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Disable" })).toBeDisabled();
    expect(
      screen.getByRole("link", { name: "Linked brokerage" }),
    ).toHaveAttribute("href", "/accounts/account-a");
    expect(screen.getByText(/0 of 3 essentials ready/)).toBeInTheDocument();
  });

  it("labels missing catalog and default preference without claiming readiness", () => {
    render(
      <ConnectionsWorkspace
        {...base}
        financialProvidersAvailable={false}
        preferenceAvailable={false}
        preference={{ connection_id: "missing", model_id: "missing" }}
      />,
    );
    expect(
      screen.getByText(/Financial providers are unavailable right now/),
    ).toBeVisible();
    expect(
      screen.queryByRole("article", { name: "Coinbase" }),
    ).not.toBeInTheDocument();
    const step = screen.getByRole("link", {
      name: /3 Default model Status unavailable/,
    });
    expect(step).not.toHaveClass("is-complete");
    expect(screen.getByText(/0 of 3 essentials ready/)).toBeInTheDocument();
    expect(screen.queryByText(/Current default:/)).not.toBeInTheDocument();
  });

  it("retains plan entitlement restrictions and explicit execution-control guidance", () => {
    render(
      <ConnectionsWorkspace
        {...base}
        financialEntitled={false}
        aiEntitled={false}
      />,
    );
    expect(
      screen.getByRole("button", { name: "Connect Coinbase" }),
    ).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Connect Charles Schwab" }),
    ).toBeDisabled();
    expect(
      screen.queryByRole("button", { name: "Add API key for OpenAI" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText(
        /Connected accounts and a selected model do not enable trading/,
      ),
    ).toBeVisible();
    expect(
      screen.getByText(/Transfer-enabled keys are rejected/),
    ).toBeVisible();
  });
});
