import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const navigation = vi.hoisted(() => ({ replace: vi.fn(), refresh: vi.fn() }));
vi.mock("next/navigation", () => ({
  useRouter: () => navigation,
}));

import { SecurityControls } from "./security-controls";

describe("SecurityControls", () => {
  afterEach(() => {
    cleanup();
    navigation.replace.mockReset();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("requires matching password confirmation before sending", () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    render(<SecurityControls />);

    fireEvent.change(screen.getByLabelText("Current password"), {
      target: { value: "correct horse battery staple" },
    });
    fireEvent.change(screen.getByLabelText("New password"), {
      target: { value: "a different secure passphrase" },
    });
    fireEvent.change(screen.getByLabelText("Confirm new password"), {
      target: { value: "does not match at all" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Change Password" }));

    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent(/does not match/i);
  });

  it("changes the password and returns to login", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(<SecurityControls />);

    fireEvent.change(screen.getByLabelText("Current password"), {
      target: { value: "correct horse battery staple" },
    });
    fireEvent.change(screen.getByLabelText("New password"), {
      target: { value: "a different secure passphrase" },
    });
    fireEvent.change(screen.getByLabelText("Confirm new password"), {
      target: { value: "a different secure passphrase" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Change Password" }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith("/api/auth/password", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        current_password: "correct horse battery staple",
        new_password: "a different secure passphrase",
      }),
    });
    expect(navigation.replace).toHaveBeenCalledWith("/login");
    expect(navigation.refresh).toHaveBeenCalledTimes(1);
  });

  it("revokes all sessions without disconnecting integrations", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(<SecurityControls />);

    fireEvent.click(
      screen.getByRole("button", { name: "Sign Out All Sessions" }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith("/api/auth/logout-all", {
      method: "POST",
    });
    expect(navigation.replace).toHaveBeenCalledWith("/login");
  });

  it("enables authenticator MFA and shows recovery codes exactly once", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          enrollment: {
            secret: "JBSWY3DPEHPK3PXP",
            otpauth_uri: "otpauth://totp/Arbion:person@example.com",
            expires_at: "2026-08-18T12:10:00Z",
          },
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          recovery_codes: ["ABCD-EFGH-JKLM-NPQR", "STUV-WXYZ-2345-6723"],
        }),
      });
    vi.stubGlobal("fetch", fetchMock);
    render(<SecurityControls />);

    fireEvent.change(screen.getByLabelText("Current password to enable MFA"), {
      target: { value: "correct horse battery staple" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Set Up Authenticator" }),
    );

    expect(await screen.findByText("JBSWY3DPEHPK3PXP")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Six-digit authenticator code"), {
      target: { value: "123456" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Verify and Enable MFA" }),
    );

    expect(await screen.findByText("ABCD-EFGH-JKLM-NPQR")).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
    expect(navigation.replace).not.toHaveBeenCalled();
    fireEvent.click(
      screen.getByRole("button", { name: "I’ve Saved My Codes" }),
    );
    expect(navigation.replace).toHaveBeenCalledWith("/login");
  });

  it("replaces recovery codes without signing out the current MFA session", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        recovery_codes: ["ABCD-EFGH-JKLM-NPQR", "STUV-WXYZ-2345-6723"],
      }),
    });
    vi.stubGlobal("fetch", fetchMock);
    render(
      <SecurityControls
        initialMFAStatus={{ enabled: true, recovery_codes_remaining: 4 }}
      />,
    );

    expect(
      screen.getByText(/4 unused recovery codes remain/i),
    ).toBeInTheDocument();
    fireEvent.change(
      screen.getByLabelText("Current password to replace codes"),
      {
        target: { value: "correct horse battery staple" },
      },
    );
    fireEvent.change(
      screen.getByLabelText("Current authenticator or recovery code"),
      { target: { value: "123456" } },
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Replace Recovery Codes" }),
    );

    expect(await screen.findByText("ABCD-EFGH-JKLM-NPQR")).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledWith("/api/auth/mfa/recovery-codes", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        current_password: "correct horse battery staple",
        code: "123456",
      }),
    });
    fireEvent.click(
      screen.getByRole("button", { name: "I’ve Saved My Codes" }),
    );
    expect(navigation.replace).not.toHaveBeenCalled();
    expect(screen.queryByText("ABCD-EFGH-JKLM-NPQR")).not.toBeInTheDocument();
  });
});
