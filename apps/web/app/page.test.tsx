import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const navigation = vi.hoisted(() => ({ push: vi.fn(), refresh: vi.fn() }));
vi.mock("next/navigation", () => ({
  useRouter: () => navigation,
}));

import { AuthForm } from "./auth-form";

describe("Authentication experience", () => {
  afterEach(() => {
    cleanup();
    navigation.push.mockReset();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("renders accessible login fields", () => {
    render(<AuthForm mode="login" />);
    expect(
      screen.getByRole("heading", { name: /welcome back/i }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/password/i)).toHaveAttribute(
      "minLength",
      "12",
    );
    expect(
      screen.getByRole("link", { name: /forgot your password/i }),
    ).toHaveAttribute("href", "/forgot-password");
  });

  it("describes registration as invite-only", () => {
    render(<AuthForm mode="register" />);
    expect(
      screen.getByRole("heading", { name: /create your invited account/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/registration is limited to invited email addresses/i),
    ).toBeInTheDocument();
  });

  it("completes a password then authenticator sign-in without creating an early session", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          mfa_required: true,
          challenge_token: "short-lived-challenge",
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ user: { id: "user-1" } }),
      });
    vi.stubGlobal("fetch", fetchMock);
    render(<AuthForm mode="login" />);

    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "person@example.com" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "correct horse battery staple" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));

    expect(
      await screen.findByRole("heading", { name: /confirm it’s you/i }),
    ).toBeInTheDocument();
    expect(navigation.push).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText(/authenticator or recovery code/i), {
      target: { value: "123456" },
    });
    fireEvent.click(screen.getByRole("button", { name: /verify and log in/i }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(fetchMock).toHaveBeenLastCalledWith("/api/auth/mfa/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        challenge_token: "short-lived-challenge",
        code: "123456",
      }),
    });
    expect(navigation.push).toHaveBeenCalledWith("/dashboard");
  });
});
