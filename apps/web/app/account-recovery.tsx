"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useState } from "react";

type RequestKind = "verification" | "password-reset";

async function apiError(response: Response) {
  const body = (await response.json().catch(() => null)) as {
    error?: { message?: string };
  } | null;
  return body?.error?.message ?? "Unable to complete the request.";
}

function fragmentToken() {
  if (typeof window === "undefined") return "";
  return new URLSearchParams(window.location.hash.slice(1)).get("token") ?? "";
}

export function EmailRequestForm({
  kind,
  initialSent = false,
}: {
  kind: RequestKind;
  initialSent?: boolean;
}) {
  const [error, setError] = useState("");
  const [message, setMessage] = useState(
    initialSent
      ? "A secure verification link has been requested. Check your inbox, or enter your email below to send a new link."
      : "",
  );
  const [busy, setBusy] = useState(false);
  const verification = kind === "verification";

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    setMessage("");
    const values = Object.fromEntries(new FormData(event.currentTarget));
    const response = await fetch(`/api/auth/${kind}/request`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(values),
    });
    if (response.ok) {
      setMessage(
        verification
          ? "If the account can be verified, a secure link will arrive shortly."
          : "If the account is eligible, a secure reset link will arrive shortly.",
      );
    } else {
      setError(await apiError(response));
    }
    setBusy(false);
  }

  return (
    <main className="auth-page">
      <section className="auth-card">
        <p className="eyebrow">ARBION</p>
        <h1>{verification ? "Verify your email." : "Reset your password."}</h1>
        <p className="lede">
          {verification
            ? "Enter your registered email and we’ll send a new secure verification link."
            : "Enter your registered email and we’ll send a secure, single-use reset link."}
        </p>
        <form onSubmit={submit}>
          <label>
            Email
            <input
              name="email"
              type="email"
              autoComplete="email"
              required
              maxLength={320}
            />
          </label>
          {error && (
            <p className="form-error" role="alert">
              {error}
            </p>
          )}
          {message && (
            <p className="form-success" role="status">
              {message}
            </p>
          )}
          <button disabled={busy}>
            {busy ? "Please wait…" : "Send secure link"}
          </button>
        </form>
        <p className="switch">
          <Link href="/login">Back to login</Link>
        </p>
      </section>
    </main>
  );
}

export function ConfirmEmailForm() {
  const [error, setError] = useState("");
  const [complete, setComplete] = useState(false);
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const token = fragmentToken();
    if (!token) {
      setError("This verification link is missing its secure token.");
      return;
    }
    setBusy(true);
    setError("");
    const response = await fetch("/api/auth/verification/confirm", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token }),
    });
    if (response.ok) setComplete(true);
    else setError(await apiError(response));
    setBusy(false);
  }

  return (
    <main className="auth-page">
      <section className="auth-card">
        <p className="eyebrow">ARBION</p>
        <h1>Verify your email.</h1>
        {complete ? (
          <>
            <p className="form-success" role="status">
              Your email is verified. You can now sign in.
            </p>
            <p className="switch">
              <Link href="/login">Continue to login</Link>
            </p>
          </>
        ) : (
          <>
            <p className="lede">
              Confirm this single-use link to activate your invited account.
            </p>
            <form onSubmit={submit}>
              {error && (
                <p className="form-error" role="alert">
                  {error}
                </p>
              )}
              <button disabled={busy}>
                {busy ? "Verifying…" : "Verify email"}
              </button>
            </form>
            <p className="switch">
              <Link href="/verify-email?request=1">Request a new link</Link>
            </p>
          </>
        )}
      </section>
    </main>
  );
}

export function ConfirmPasswordResetForm() {
  const router = useRouter();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const token = fragmentToken();
    const values = new FormData(event.currentTarget);
    const password = String(values.get("new_password") ?? "");
    if (!token) {
      setError("This reset link is missing its secure token.");
      return;
    }
    if (password !== String(values.get("confirm_password") ?? "")) {
      setError("Passwords do not match.");
      return;
    }
    setBusy(true);
    setError("");
    const response = await fetch("/api/auth/password-reset/confirm", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token, new_password: password }),
    });
    if (response.ok) {
      router.push("/login?password_reset=1");
      router.refresh();
      return;
    }
    setError(await apiError(response));
    setBusy(false);
  }

  return (
    <main className="auth-page">
      <section className="auth-card">
        <p className="eyebrow">ARBION</p>
        <h1>Choose a new password.</h1>
        <p className="lede">
          Completing this reset signs out every existing Arbion session.
        </p>
        <form onSubmit={submit}>
          <label>
            New password
            <input
              name="new_password"
              type="password"
              autoComplete="new-password"
              required
              minLength={12}
              maxLength={1024}
            />
          </label>
          <label>
            Confirm new password
            <input
              name="confirm_password"
              type="password"
              autoComplete="new-password"
              required
              minLength={12}
              maxLength={1024}
            />
          </label>
          {error && (
            <p className="form-error" role="alert">
              {error}
            </p>
          )}
          <button disabled={busy}>
            {busy ? "Resetting…" : "Reset password"}
          </button>
        </form>
      </section>
    </main>
  );
}
