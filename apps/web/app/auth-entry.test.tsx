import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const navigation = vi.hoisted(() => ({ push: vi.fn(), refresh: vi.fn() }));
vi.mock("next/navigation", () => ({ useRouter: () => navigation }));

import { AuthForm } from "./auth-form";
import {
  ConfirmEmailForm,
  ConfirmPasswordResetForm,
  EmailRequestForm,
} from "./account-recovery";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
  window.location.hash = "";
});

function reply(body: object = {}, status = 200) {
  return new Response(JSON.stringify(body), { status });
}
function pendingReply() {
  let resolve!: (response: Response) => void;
  const response = new Promise<Response>((done) => {
    resolve = done;
  });
  return { response, resolve };
}
function enterPasswordLogin() {
  fireEvent.change(screen.getByLabelText("Email"), {
    target: { value: "person@example.com" },
  });
  fireEvent.change(screen.getByLabelText("Password"), {
    target: { value: "illustrative-password-only" },
  });
}

describe("shared authentication entry presentation", () => {
  it.each([
    {
      view: <AuthForm mode="login" />,
      heading: "Welcome back.",
      step: "Account access",
    },
    {
      view: <AuthForm mode="register" />,
      heading: "Create your invited account.",
      step: "Invited access",
    },
    {
      view: <EmailRequestForm kind="password-reset" />,
      heading: "Reset your password.",
      step: "Account recovery",
    },
    {
      view: <EmailRequestForm kind="verification" />,
      heading: "Verify your email.",
      step: "Email verification",
    },
    {
      view: <EmailRequestForm kind="verification" initialSent />,
      heading: "Check your inbox.",
      step: "Email verification",
    },
    {
      view: <ConfirmEmailForm />,
      heading: "Verify your email.",
      step: "Email verification",
    },
    {
      view: <ConfirmPasswordResetForm />,
      heading: "Choose a new password.",
      step: "Account recovery",
    },
  ])(
    "keeps $heading branded, named and inert on render",
    ({ view, heading, step }) => {
      const fetch = vi.fn();
      vi.stubGlobal("fetch", fetch);
      render(view);
      expect(
        screen.getByRole("heading", { level: 1, name: heading }),
      ).toHaveAttribute("id", "auth-entry-title");
      expect(screen.getByRole("region", { name: heading })).toHaveClass(
        "auth-card",
      );
      expect(screen.getByText(step)).toHaveClass("eyebrow");
      expect(screen.getByRole("link", { name: "Arbion home" })).toHaveAttribute(
        "href",
        "/",
      );
      expect(screen.getByRole("img", { name: "Arbion" })).toBeInTheDocument();
      expect(document.querySelectorAll("#auth-entry-title")).toHaveLength(1);
      expect(fetch).not.toHaveBeenCalled();
      expect(navigation.push).not.toHaveBeenCalled();
      expect(navigation.refresh).not.toHaveBeenCalled();
    },
  );

  it.each(["login", "register"] as const)(
    "preserves native %s fields and password-manager semantics",
    (mode) => {
      render(<AuthForm mode={mode} />);
      const email = screen.getByLabelText("Email");
      expect(email).toHaveAttribute("name", "email");
      expect(email).toHaveAttribute("type", "email");
      expect(email).toHaveAttribute("autocomplete", "email");
      expect(email).toHaveAttribute("maxlength", "320");
      expect(email).toBeRequired();
      const password = screen.getByLabelText("Password");
      expect(password).toHaveAttribute("name", "password");
      expect(password).toHaveAttribute("type", "password");
      expect(password).toHaveAttribute(
        "autocomplete",
        mode === "login" ? "current-password" : "new-password",
      );
      expect(password).toHaveAttribute("minlength", "12");
      expect(password).toHaveAttribute("maxlength", "1024");
      expect(password).toBeRequired();
      if (mode === "register") {
        const name = screen.getByLabelText("Display name");
        expect(name).toHaveAttribute("name", "display_name");
        expect(name).toHaveAttribute("autocomplete", "name");
        expect(name).toHaveAttribute("maxlength", "100");
        expect(name).not.toBeRequired();
        expect(
          screen.getByText(
            "Registration is limited to invited email addresses.",
          ),
        ).toBeInTheDocument();
      }
    },
  );

  it("preserves the exact password request, pending state and server error without navigating", async () => {
    const pending = pendingReply();
    const request = vi.fn(() => pending.response);
    vi.stubGlobal("fetch", request);
    render(<AuthForm mode="login" />);
    enterPasswordLogin();
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));
    expect(screen.getByRole("button", { name: "Please wait…" })).toBeDisabled();
    expect(request).toHaveBeenCalledExactlyOnceWith("/api/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        email: "person@example.com",
        password: "illustrative-password-only",
      }),
    });
    pending.resolve(
      reply({ error: { message: "Illustrative sign-in error" } }, 401),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Illustrative sign-in error",
    );
    expect(screen.getByRole("button", { name: "Log in" })).not.toBeDisabled();
    expect(screen.getByLabelText("Email")).toHaveValue("person@example.com");
    expect(navigation.push).not.toHaveBeenCalled();
  });

  it("preserves invited registration payload and verification handoff", async () => {
    const request = vi.fn(async () => reply({ verification_required: true }));
    vi.stubGlobal("fetch", request);
    render(<AuthForm mode="register" />);
    enterPasswordLogin();
    fireEvent.change(screen.getByLabelText("Display name"), {
      target: { value: "Illustrative owner" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Register" }));
    await waitFor(() =>
      expect(navigation.push).toHaveBeenCalledExactlyOnceWith(
        "/verify-email?sent=1",
      ),
    );
    expect(request).toHaveBeenCalledExactlyOnceWith("/api/auth/register", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        display_name: "Illustrative owner",
        email: "person@example.com",
        password: "illustrative-password-only",
      }),
    });
    expect(navigation.refresh).not.toHaveBeenCalled();
  });

  it("preserves the MFA request boundary, failed-code state and explicit restart", async () => {
    const pending = pendingReply();
    const request = vi
      .fn()
      .mockResolvedValueOnce(
        reply({
          mfa_required: true,
          challenge_token: "illustrative-challenge",
        }),
      )
      .mockReturnValueOnce(pending.response);
    vi.stubGlobal("fetch", request);
    render(<AuthForm mode="login" />);
    enterPasswordLogin();
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));
    expect(await screen.findByText("Identity check")).toBeInTheDocument();
    expect(
      screen.getByRole("region", { name: "Confirm it’s you." }),
    ).toBeInTheDocument();
    const code = screen.getByLabelText("Authenticator or recovery code");
    expect(code).toHaveAttribute("autocomplete", "one-time-code");
    expect(code).toHaveAttribute("maxlength", "32");
    expect(code).toHaveAttribute("inputmode", "text");
    expect(code).toHaveFocus();
    expect(code).toBeRequired();
    expect(screen.queryByLabelText("Password")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("link", { name: "Create your account" }),
    ).not.toBeInTheDocument();
    fireEvent.change(code, { target: { value: "illustrative-recovery-code" } });
    fireEvent.click(screen.getByRole("button", { name: "Verify and Log In" }));
    expect(screen.getByRole("button", { name: "Verifying…" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Start Over" })).toBeDisabled();
    expect(request).toHaveBeenLastCalledWith("/api/auth/mfa/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        challenge_token: "illustrative-challenge",
        code: "illustrative-recovery-code",
      }),
    });
    pending.resolve(
      reply({ error: { message: "Illustrative invalid code" } }, 401),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Illustrative invalid code",
    );
    expect(navigation.push).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Start Over" }));
    expect(
      screen.getByRole("heading", { name: "Welcome back." }),
    ).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(request).toHaveBeenCalledTimes(2);
  });

  it.each(["verification", "password-reset"] as const)(
    "preserves exact %s requests and non-enumerating acknowledgement",
    async (kind) => {
      const pending = pendingReply();
      const request = vi.fn(() => pending.response);
      vi.stubGlobal("fetch", request);
      render(<EmailRequestForm kind={kind} />);
      fireEvent.change(screen.getByLabelText("Email"), {
        target: { value: "person@example.com" },
      });
      fireEvent.click(screen.getByRole("button", { name: "Send secure link" }));
      expect(
        screen.getByRole("button", { name: "Please wait…" }),
      ).toBeDisabled();
      expect(request).toHaveBeenCalledExactlyOnceWith(
        `/api/auth/${kind}/request`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ email: "person@example.com" }),
        },
      );
      pending.resolve(reply({}, 202));
      expect(await screen.findByRole("status")).toHaveTextContent(
        kind === "verification"
          ? "If the account can be verified, a secure link will arrive shortly."
          : "If the account is eligible, a secure reset link will arrive shortly.",
      );
      expect(
        screen.getByRole("button", { name: "Send secure link" }),
      ).not.toBeDisabled();
      expect(navigation.push).not.toHaveBeenCalled();
    },
  );

  it("preserves the missing verification-token guard without sending a request", () => {
    const request = vi.fn();
    vi.stubGlobal("fetch", request);
    render(<ConfirmEmailForm />);
    fireEvent.click(screen.getByRole("button", { name: "Verify email" }));
    expect(screen.getByRole("alert")).toHaveTextContent(
      "This verification link is missing its secure token.",
    );
    expect(request).not.toHaveBeenCalled();
    expect(
      screen.getByRole("link", { name: "Request a new link" }),
    ).toHaveAttribute("href", "/verify-email?request=1");
  });

  it("preserves exact reset fields, session warning and success redirect", async () => {
    window.location.hash = "token=illustrative-fragment-token";
    const pending = pendingReply();
    const request = vi.fn(() => pending.response);
    vi.stubGlobal("fetch", request);
    render(<ConfirmPasswordResetForm />);
    expect(
      screen.getByText(
        "Completing this reset signs out every existing Arbion session.",
      ),
    ).toBeInTheDocument();
    for (const name of ["New password", "Confirm new password"]) {
      const field = screen.getByLabelText(name);
      expect(field).toHaveAttribute("type", "password");
      expect(field).toHaveAttribute("autocomplete", "new-password");
      expect(field).toHaveAttribute("minlength", "12");
      expect(field).toHaveAttribute("maxlength", "1024");
      expect(field).toBeRequired();
      fireEvent.change(field, {
        target: { value: "illustrative-password-only" },
      });
    }
    expect(screen.getByRole("link", { name: "Back to login" })).toHaveAttribute(
      "href",
      "/login",
    );
    fireEvent.click(screen.getByRole("button", { name: "Reset password" }));
    expect(screen.getByRole("button", { name: "Resetting…" })).toBeDisabled();
    expect(request).toHaveBeenCalledExactlyOnceWith(
      "/api/auth/password-reset/confirm",
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          token: "illustrative-fragment-token",
          new_password: "illustrative-password-only",
        }),
      },
    );
    pending.resolve(reply());
    await waitFor(() =>
      expect(navigation.push).toHaveBeenCalledExactlyOnceWith(
        "/login?password_reset=1",
      ),
    );
    expect(navigation.refresh).toHaveBeenCalledTimes(1);
  });
});
