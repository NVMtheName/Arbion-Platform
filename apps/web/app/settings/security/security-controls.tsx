"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

type ErrorBody = { error?: { message?: string } };

async function message(response: Response) {
  const body = (await response.json().catch(() => null)) as ErrorBody | null;
  return body?.error?.message ?? "Unable to complete the request.";
}

export function SecurityControls() {
  const router = useRouter();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState<"password" | "sessions" | "">("");

  function returnToLogin() {
    router.replace("/login");
    router.refresh();
  }

  async function changePassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    const form = event.currentTarget;
    const values = new FormData(form);
    const currentPassword = String(values.get("current_password") ?? "");
    const newPassword = String(values.get("new_password") ?? "");
    const confirmation = String(values.get("confirm_password") ?? "");
    if (newPassword !== confirmation) {
      setError("New password confirmation does not match.");
      return;
    }
    setBusy("password");
    const response = await fetch("/api/auth/password", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        current_password: currentPassword,
        new_password: newPassword,
      }),
    });
    if (response.ok) {
      form.reset();
      returnToLogin();
      return;
    }
    setError(await message(response));
    setBusy("");
  }

  async function logoutEverywhere() {
    setError("");
    setBusy("sessions");
    const response = await fetch("/api/auth/logout-all", { method: "POST" });
    if (response.ok) {
      returnToLogin();
      return;
    }
    setError(await message(response));
    setBusy("");
  }

  return (
    <div className="security-grid">
      <section className="security-card" aria-labelledby="password-title">
        <p className="eyebrow">PASSWORD</p>
        <h2 id="password-title">Change password</h2>
        <p>
          Changing your password signs out every Arbion session, including this
          browser. Your financial and AI connections remain intact.
        </p>
        <form onSubmit={changePassword}>
          <label>
            Current password
            <input
              name="current_password"
              type="password"
              autoComplete="current-password"
              required
              maxLength={1024}
            />
          </label>
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
          <button disabled={busy !== ""}>
            {busy === "password" ? "Changing…" : "Change Password"}
          </button>
        </form>
      </section>
      <section className="security-card" aria-labelledby="sessions-title">
        <p className="eyebrow">SESSIONS</p>
        <h2 id="sessions-title">Sign out everywhere</h2>
        <p>
          Immediately revoke every Arbion browser session for your account. This
          does not disconnect Schwab or disable automations.
        </p>
        <button
          className="danger"
          disabled={busy !== ""}
          onClick={logoutEverywhere}
        >
          {busy === "sessions" ? "Signing out…" : "Sign Out All Sessions"}
        </button>
      </section>
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}
