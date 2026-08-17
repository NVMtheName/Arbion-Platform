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
});
