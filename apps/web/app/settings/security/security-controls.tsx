"use client";

import { FormEvent, useState } from "react";
import { useRouter } from "next/navigation";

type ErrorBody = { error?: { message?: string } };
type MFAStatus = { enabled: boolean; recovery_codes_remaining: number };
type Enrollment = { secret: string; otpauth_uri: string; expires_at: string };

async function message(response: Response) {
  const body = (await response.json().catch(() => null)) as ErrorBody | null;
  return body?.error?.message ?? "Unable to complete the request.";
}

export function SecurityControls({
  initialMFAStatus = { enabled: false, recovery_codes_remaining: 0 },
}: {
  initialMFAStatus?: MFAStatus;
}) {
  const router = useRouter();
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const [busy, setBusy] = useState<
    | "password"
    | "sessions"
    | "mfa-enroll"
    | "mfa-confirm"
    | "mfa-regenerate"
    | "mfa-disable"
    | ""
  >("");
  const [mfa, setMFA] = useState(initialMFAStatus);
  const [enrollment, setEnrollment] = useState<Enrollment | null>(null);
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([]);
  const [recoveryRequiresLogin, setRecoveryRequiresLogin] = useState(false);

  function clearMessages() {
    setError("");
    setSuccess("");
  }

  function returnToLogin() {
    router.replace("/login");
    router.refresh();
  }

  async function changePassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    clearMessages();
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
    clearMessages();
    setBusy("sessions");
    const response = await fetch("/api/auth/logout-all", { method: "POST" });
    if (response.ok) {
      returnToLogin();
      return;
    }
    setError(await message(response));
    setBusy("");
  }

  async function beginMFA(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    clearMessages();
    setBusy("mfa-enroll");
    const form = event.currentTarget;
    const values = new FormData(form);
    const response = await fetch("/api/auth/mfa/totp/enroll", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        current_password: String(values.get("current_password") ?? ""),
      }),
    });
    if (response.ok) {
      const body = (await response.json()) as { enrollment: Enrollment };
      form.reset();
      setEnrollment(body.enrollment);
      setBusy("");
      return;
    }
    setError(await message(response));
    setBusy("");
  }

  async function confirmMFA(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    clearMessages();
    setBusy("mfa-confirm");
    const form = event.currentTarget;
    const values = new FormData(form);
    const response = await fetch("/api/auth/mfa/totp/confirm", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code: String(values.get("code") ?? "") }),
    });
    if (response.ok) {
      const body = (await response.json()) as { recovery_codes: string[] };
      setRecoveryCodes(body.recovery_codes);
      setRecoveryRequiresLogin(true);
      setMFA({
        enabled: true,
        recovery_codes_remaining: body.recovery_codes.length,
      });
      setEnrollment(null);
      setSuccess(
        "Authenticator MFA is enabled. Save every recovery code before continuing.",
      );
      setBusy("");
      return;
    }
    setError(await message(response));
    setBusy("");
  }

  async function regenerateRecoveryCodes(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    clearMessages();
    setBusy("mfa-regenerate");
    const form = event.currentTarget;
    const values = new FormData(form);
    const response = await fetch("/api/auth/mfa/recovery-codes", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        current_password: String(values.get("current_password") ?? ""),
        code: String(values.get("code") ?? ""),
      }),
    });
    if (response.ok) {
      const body = (await response.json()) as { recovery_codes: string[] };
      form.reset();
      setRecoveryCodes(body.recovery_codes);
      setRecoveryRequiresLogin(false);
      setMFA({
        enabled: true,
        recovery_codes_remaining: body.recovery_codes.length,
      });
      setSuccess(
        "Your old recovery codes are invalid. Save the replacement codes now.",
      );
      setBusy("");
      return;
    }
    setError(await message(response));
    setBusy("");
  }

  function finishRecoveryCodes() {
    if (recoveryRequiresLogin) {
      returnToLogin();
      return;
    }
    setRecoveryCodes([]);
    setSuccess("Replacement recovery codes saved.");
  }

  async function disableMFA(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    clearMessages();
    setBusy("mfa-disable");
    const values = new FormData(event.currentTarget);
    const response = await fetch("/api/auth/mfa/totp", {
      method: "DELETE",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        current_password: String(values.get("current_password") ?? ""),
        code: String(values.get("code") ?? ""),
      }),
    });
    if (response.ok) {
      returnToLogin();
      return;
    }
    setError(await message(response));
    setBusy("");
  }

  return (
    <div className="security-grid">
      <section className="security-card" aria-labelledby="mfa-title">
        <p className="eyebrow">AUTHENTICATOR MFA</p>
        <h2 id="mfa-title">
          {mfa.enabled
            ? "Extra sign-in protection is on"
            : "Add an authenticator app"}
        </h2>
        {recoveryCodes.length > 0 ? (
          <>
            <p>
              Store these one-time codes in a password manager. Arbion stores
              only their hashes, so they cannot be shown again.
            </p>
            <ul className="recovery-code-list" aria-label="Recovery codes">
              {recoveryCodes.map((code) => (
                <li key={code}>
                  <code>{code}</code>
                </li>
              ))}
            </ul>
            <button onClick={finishRecoveryCodes}>I’ve Saved My Codes</button>
          </>
        ) : enrollment ? (
          <>
            <p>
              Add a new time-based account in your authenticator app using this
              setup key. The key expires from this setup flow in ten minutes.
            </p>
            <p className="mfa-setup-key">
              <span>Setup key</span>
              <code>{enrollment.secret}</code>
            </p>
            <a className="button-link secondary" href={enrollment.otpauth_uri}>
              Open Authenticator App
            </a>
            <form onSubmit={confirmMFA}>
              <label>
                Six-digit authenticator code
                <input
                  name="code"
                  autoComplete="one-time-code"
                  inputMode="numeric"
                  pattern="[0-9]{6}"
                  minLength={6}
                  maxLength={6}
                  required
                />
              </label>
              <button disabled={busy !== ""}>
                {busy === "mfa-confirm"
                  ? "Verifying…"
                  : "Verify and Enable MFA"}
              </button>
            </form>
          </>
        ) : mfa.enabled ? (
          <>
            <p>
              Sign-in requires your password plus an authenticator or recovery
              code. {mfa.recovery_codes_remaining} unused recovery codes remain.
            </p>
            <form onSubmit={disableMFA}>
              <label>
                Current password to disable MFA
                <input
                  name="current_password"
                  type="password"
                  autoComplete="current-password"
                  required
                  maxLength={1024}
                />
              </label>
              <label>
                Authenticator or recovery code
                <input
                  name="code"
                  autoComplete="one-time-code"
                  required
                  maxLength={32}
                />
              </label>
              <button className="danger" disabled={busy !== ""}>
                {busy === "mfa-disable" ? "Disabling…" : "Disable MFA"}
              </button>
            </form>
            <div className="security-subsection">
              <h3>Replace recovery codes</h3>
              <p>
                Generate a fresh set if a code was exposed or lost. Every old
                recovery code becomes invalid immediately.
              </p>
              <form onSubmit={regenerateRecoveryCodes}>
                <label>
                  Current password to replace codes
                  <input
                    name="current_password"
                    type="password"
                    autoComplete="current-password"
                    required
                    maxLength={1024}
                  />
                </label>
                <label>
                  Current authenticator or recovery code
                  <input
                    name="code"
                    autoComplete="one-time-code"
                    required
                    maxLength={32}
                  />
                </label>
                <button className="secondary" disabled={busy !== ""}>
                  {busy === "mfa-regenerate"
                    ? "Replacing…"
                    : "Replace Recovery Codes"}
                </button>
              </form>
            </div>
          </>
        ) : (
          <>
            <p>
              Use any standards-based authenticator app. Setup verifies your
              current password and signs out existing sessions when MFA is
              enabled.
            </p>
            <form onSubmit={beginMFA}>
              <label>
                Current password to enable MFA
                <input
                  name="current_password"
                  type="password"
                  autoComplete="current-password"
                  required
                  maxLength={1024}
                />
              </label>
              <button disabled={busy !== ""}>
                {busy === "mfa-enroll" ? "Preparing…" : "Set Up Authenticator"}
              </button>
            </form>
          </>
        )}
      </section>

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

      {success && (
        <p className="form-success" role="status">
          {success}
        </p>
      )}
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}
