import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  SecurityActivity,
  type SecurityActivityRecord,
} from "./security-activity";

const login: SecurityActivityRecord = {
  id: "11111111-1111-4111-8111-111111111111",
  action: "auth.login",
  occurred_at: "2026-08-28T01:30:00Z",
};

describe("SecurityActivity", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("renders safe access, connection, autonomy, and control events", () => {
    render(
      <SecurityActivity
        initialActivities={[
          login,
          {
            ...login,
            id: "22222222-2222-4222-8222-222222222222",
            action: "financial.connection_disabled",
          },
          {
            ...login,
            id: "33333333-3333-4333-8333-333333333333",
            action: "automation_mandate.autonomy_changed",
          },
          {
            ...login,
            id: "44444444-4444-4444-8444-444444444444",
            action: "global_circuit_breaker.engaged",
          },
        ]}
      />,
    );

    expect(screen.getByText("Successful sign-in")).toBeInTheDocument();
    expect(
      screen.getByText("Financial connection disabled"),
    ).toBeInTheDocument();
    expect(screen.getByText("Autonomy policy changed")).toBeInTheDocument();
    expect(
      screen.getByText("Global emergency stop engaged"),
    ).toBeInTheDocument();
    expect(screen.getByText(/excludes email addresses/i)).toBeInTheDocument();
    expect(screen.queryByText(/credential value/i)).not.toBeInTheDocument();
  });

  it("loads an older page without duplicating existing events", async () => {
    const earlier: SecurityActivityRecord = {
      ...login,
      id: "55555555-5555-4555-8555-555555555555",
      action: "auth.mfa_enabled",
    };
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ activities: [login, earlier], next_cursor: "" }),
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <SecurityActivity
        initialActivities={[login]}
        initialCursor="older-cursor"
      />,
    );

    fireEvent.click(
      screen.getByRole("button", { name: "Load earlier activity" }),
    );
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(String(fetchMock.mock.calls[0][0])).toContain(
      "/api/auth/security-activity?limit=20&cursor=older-cursor",
    );
    expect(
      await screen.findByText("Authenticator MFA enabled"),
    ).toBeInTheDocument();
    expect(screen.getAllByText("Successful sign-in")).toHaveLength(1);
  });

  it("fails visibly when activity cannot be verified", () => {
    render(<SecurityActivity initialActivities={[]} available={false} />);
    expect(
      screen.getByText("Security activity could not be verified."),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/password and MFA controls remain available/i),
    ).toBeInTheDocument();
  });
});
