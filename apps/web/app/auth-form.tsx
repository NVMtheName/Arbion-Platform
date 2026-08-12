"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { FormEvent, useState } from "react";

export function AuthForm({ mode }: { mode: "login" | "register" }) {
  const router = useRouter();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    const values = Object.fromEntries(new FormData(event.currentTarget));
    const response = await fetch(`/api/auth/${mode}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(values),
    });
    if (response.ok) {
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
  return (
    <main className="auth-page">
      <section className="auth-card">
        <p className="eyebrow">ARBION</p>
        <h1>{mode === "login" ? "Welcome back." : "Create your account."}</h1>
        <p className="lede">
          A secure workspace for disciplined financial decisions.
        </p>
        <form onSubmit={submit}>
          {mode === "register" && (
            <label>
              Display name
              <input name="display_name" autoComplete="name" maxLength={100} />
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
        <p className="switch">
          {mode === "login" ? "New to Arbion?" : "Already registered?"}{" "}
          <Link href={mode === "login" ? "/register" : "/login"}>
            {mode === "login" ? "Create an account" : "Log in"}
          </Link>
        </p>
      </section>
    </main>
  );
}
