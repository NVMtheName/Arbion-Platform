import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const push = vi.fn();
const refresh = vi.fn();
vi.mock("next/navigation", () => ({ useRouter: () => ({ push, refresh }) }));

import {
  ConfirmEmailForm,
  ConfirmPasswordResetForm,
  EmailRequestForm,
} from "./account-recovery";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  push.mockReset();
  refresh.mockReset();
  window.location.hash = "";
});

describe("Account recovery", () => {
  it("shows an Arbion-branded verification handoff after registration", () => {
    render(<EmailRequestForm kind="verification" initialSent />);
    expect(screen.getByRole("img", { name: "Arbion" })).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: /check your inbox/i }),
    ).toBeInTheDocument();
    expect(screen.getByRole("note")).toHaveTextContent(
      /one final secure step/i,
    );
    expect(
      screen.getByRole("button", { name: /send another link/i }),
    ).toBeInTheDocument();
  });

  it("shows the same password-reset acknowledgement without exposing account state", async () => {
    const request = vi
      .fn()
      .mockResolvedValue(new Response(null, { status: 202 }));
    vi.stubGlobal("fetch", request);
    render(<EmailRequestForm kind="password-reset" />);
    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "person@example.com" },
    });
    fireEvent.click(screen.getByRole("button", { name: /send secure link/i }));
    expect(await screen.findByRole("status")).toHaveTextContent(
      /if the account is eligible/i,
    );
    expect(request).toHaveBeenCalledWith(
      "/api/auth/password-reset/request",
      expect.objectContaining({ method: "POST" }),
    );
  });

  it("submits a verification token from the URL fragment", async () => {
    window.location.hash = "token=abcdefghijklmnopqrstuvwxyzABCDEFGH123456789";
    const request = vi
      .fn()
      .mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", request);
    render(<ConfirmEmailForm />);
    fireEvent.click(screen.getByRole("button", { name: /verify email/i }));
    expect(await screen.findByRole("status")).toHaveTextContent(
      /email is verified/i,
    );
    const payload = JSON.parse(String(request.mock.calls[0][1].body)) as {
      token: string;
    };
    expect(payload.token).toBe("abcdefghijklmnopqrstuvwxyzABCDEFGH123456789");
  });

  it("validates matching passwords before confirming a reset", async () => {
    window.location.hash = "token=abcdefghijklmnopqrstuvwxyzABCDEFGH123456789";
    const request = vi.fn();
    vi.stubGlobal("fetch", request);
    render(<ConfirmPasswordResetForm />);
    fireEvent.change(screen.getByLabelText("New password"), {
      target: { value: "a sufficiently long password" },
    });
    fireEvent.change(screen.getByLabelText("Confirm new password"), {
      target: { value: "a different long password" },
    });
    fireEvent.click(screen.getByRole("button", { name: /reset password/i }));
    expect(await screen.findByRole("alert")).toHaveTextContent(/do not match/i);
    await waitFor(() => expect(request).not.toHaveBeenCalled());
  });
});
