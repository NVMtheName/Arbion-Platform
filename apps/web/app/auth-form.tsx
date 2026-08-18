"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useState } from "react";

import { ArbionBrand } from "./brand";

export function AuthForm({ mode }: { mode: "login" | "register" }) {
  const router = useRouter();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [challengeToken, setChallengeToken] = useState("");
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    const values = Object.fromEntries(new FormData(event.currentTarget));
    const response = await fetch(
      challengeToken ? "/api/auth/mfa/login" : `/api/auth/${mode}`,
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(
          challengeToken
            ? { challenge_token: challengeToken, code: values.code }
            : values,
        ),
      },
    );
    if (response.ok) {
      const body = (await response.json().catch(() => null)) as {
        verification_required?: boolean;
        mfa_required?: boolean;
        challenge_token?: string;
      } | null;
      if (
        mode === "login" &&
        body?.mfa_required &&
        typeof body.challenge_token === "string" &&
        body.challenge_token !== ""
      ) {
        setChallengeToken(body.challenge_token);
        setBusy(false);
        return;
      }
      if (mode === "register" && body?.verification_required) {
        router.push("/verify-email?sent=1");
        return;
      }
      router.push("/dashboard");
      router.refresh();
      return;
    }
    const body = (await response.json().catch(() => null)) as {
      error?: { message?: string };
    } | null;
    setError(body?.error?.message ?? "Unable to complete the request.");
    setBusy(false);
  }

  function restartLogin() {
    setChallengeToken("");
    setError("");
  }

  return (
    <main className="auth-page">
      <section className="auth-card">
        <ArbionBrand className="auth-brand" priority />
        <h1>
          {challengeToken
            ? "Confirm it’s you."
            : mode === "login"
              ? "Welcome back."
              : "Create your invited account."}
        </h1>
        <p className="lede">
          {challengeToken
            ? "Enter the six-digit code from your authenticator app, or use one of your recovery codes."
            : mode === "login"
              ? "A secure workspace for disciplined financial decisions."
              : "Registration is limited to invited email addresses."}
        </p>
        {challengeToken ? (
          <form onSubmit={submit}>
            <label>
              Authenticator or recovery code
              <input
                name="code"
                autoComplete="one-time-code"
                inputMode="text"
                required
                maxLength={32}
                autoFocus
              />
            </label>
            {error && (
              <p className="form-error" role="alert">
                {error}
              </p>
            )}
            <button disabled={busy}>
              {busy ? "Verifying…" : "Verify and Log In"}
            </button>
            <button
              className="secondary"
              type="button"
              disabled={busy}
              onClick={restartLogin}
            >
              Start Over
            </button>
          </form>
        ) : (
          <form onSubmit={submit}>
            {mode === "register" && (
              <label>
                Display name
                <input
                  name="display_name"
                  autoComplete="name"
                  maxLength={100}
                />
              </label>
            )}
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
            <label>
              Password
              <input
                name="password"
                type="password"
                autoComplete={
                  mode === "login" ? "current-password" : "new-password"
                }
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
              {busy ? "Please wait…" : mode === "login" ? "Log in" : "Register"}
            </button>
          </form>
        )}
        {!challengeToken && (
          <>
            <p className="switch">
              {mode === "login" ? "Have an invite?" : "Already registered?"}{" "}
              <Link href={mode === "login" ? "/register" : "/login"}>
                {mode === "login" ? "Create your account" : "Log in"}
              </Link>
            </p>
            {mode === "login" && (
              <p className="switch">
                <Link href="/forgot-password">Forgot your password?</Link>
                {" · "}
                <Link href="/verify-email?request=1">
                  Resend verification email
                </Link>
              </p>
            )}
          </>
        )}
      </section>
    </main>
  );
}
