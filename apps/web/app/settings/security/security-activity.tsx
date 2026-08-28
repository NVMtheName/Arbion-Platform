"use client";

import { useState } from "react";

export type SecurityActivityRecord = {
  id: string;
  action: string;
  occurred_at: string;
};

type Presentation = {
  category: string;
  title: string;
  detail: string;
  tone: "access" | "connection" | "control" | "authority";
};

const presentations: Record<string, Presentation> = {
  "auth.registration": {
    category: "Access",
    title: "Account registered",
    detail: "The Arbion account was created.",
    tone: "access",
  },
  "auth.login": {
    category: "Access",
    title: "Successful sign-in",
    detail: "A new authenticated session was created.",
    tone: "access",
  },
  "auth.login_mfa_required": {
    category: "Access",
    title: "Authenticator challenge issued",
    detail: "A password sign-in required the configured second factor.",
    tone: "access",
  },
  "auth.logout": {
    category: "Access",
    title: "Session signed out",
    detail: "One authenticated session was revoked.",
    tone: "access",
  },
  "auth.logout_others": {
    category: "Access",
    title: "Other sessions signed out",
    detail:
      "Every other browser session was revoked; this session stayed active.",
    tone: "access",
  },
  "auth.logout_all": {
    category: "Access",
    title: "All sessions signed out",
    detail: "Every active session and pending sign-in challenge was revoked.",
    tone: "access",
  },
  "auth.password_change_failed": {
    category: "Access",
    title: "Password change rejected",
    detail: "A password change failed closed.",
    tone: "access",
  },
  "auth.password_changed": {
    category: "Access",
    title: "Password changed",
    detail: "The password was replaced and existing sessions were revoked.",
    tone: "access",
  },
  "auth.password_reset": {
    category: "Access",
    title: "Password reset completed",
    detail: "A single-use reset was completed and sessions were revoked.",
    tone: "access",
  },
  "auth.email_verified": {
    category: "Access",
    title: "Email verified",
    detail: "The account email verification was completed.",
    tone: "access",
  },
  "auth.email_delivery_failed": {
    category: "Access",
    title: "Security email delivery failed",
    detail: "A security email was not delivered; no account control changed.",
    tone: "access",
  },
  "auth.email_token_requested": {
    category: "Access",
    title: "Security email requested",
    detail: "A single-use account verification or recovery email was sent.",
    tone: "access",
  },
  "auth.mfa_enrollment_started": {
    category: "Access",
    title: "Authenticator setup started",
    detail: "A time-limited authenticator enrollment was created.",
    tone: "access",
  },
  "auth.mfa_enabled": {
    category: "Access",
    title: "Authenticator MFA enabled",
    detail: "Second-factor protection was enabled and sessions were revoked.",
    tone: "access",
  },
  "auth.mfa_disabled": {
    category: "Access",
    title: "Authenticator MFA disabled",
    detail: "Second-factor protection was removed after step-up verification.",
    tone: "access",
  },
  "auth.mfa_recovery_codes_replaced": {
    category: "Access",
    title: "Recovery codes replaced",
    detail: "Prior recovery codes were invalidated.",
    tone: "access",
  },
  "auth.mfa_login_failed": {
    category: "Access",
    title: "Authenticator sign-in rejected",
    detail: "An invalid or expired second-factor attempt failed closed.",
    tone: "access",
  },
  "ai_connection.created": {
    category: "Connection",
    title: "AI provider connected",
    detail: "A new encrypted AI credential connection was created.",
    tone: "connection",
  },
  "ai_connection.credential_replaced": {
    category: "Connection",
    title: "AI credential replaced",
    detail: "The encrypted provider credential was rotated.",
    tone: "connection",
  },
  "ai_connection.deleted": {
    category: "Connection",
    title: "AI connection deleted",
    detail: "An unreferenced AI connection and its Vault secret were removed.",
    tone: "connection",
  },
  "ai_connection.display_name_changed": {
    category: "Connection",
    title: "AI connection renamed",
    detail: "The connection label changed without changing its credential.",
    tone: "connection",
  },
  "ai_connection.disabled": {
    category: "Connection",
    title: "AI connection disabled",
    detail: "The connection became ineligible for model use.",
    tone: "connection",
  },
  "ai_connection.enabled": {
    category: "Connection",
    title: "AI connection enabled",
    detail: "The connection returned to pending verification eligibility.",
    tone: "connection",
  },
  "ai_connection.verification_failed": {
    category: "Connection",
    title: "AI verification failed",
    detail: "Provider verification failed without exposing its response.",
    tone: "connection",
  },
  "ai_connection.verification_succeeded": {
    category: "Connection",
    title: "AI connection verified",
    detail: "The provider accepted the encrypted credential.",
    tone: "connection",
  },
  "financial.authorization_started": {
    category: "Connection",
    title: "Financial authorization started",
    detail: "A provider authorization flow was initiated.",
    tone: "connection",
  },
  "financial.authorization_completed": {
    category: "Connection",
    title: "Financial account connected",
    detail: "A financial provider authorization completed successfully.",
    tone: "connection",
  },
  "financial.authorization_failed": {
    category: "Connection",
    title: "Financial authorization failed",
    detail: "The provider authorization failed closed.",
    tone: "connection",
  },
  "financial.connection_disabled": {
    category: "Connection",
    title: "Financial connection disabled",
    detail:
      "New use of the connection was stopped without changing broker state.",
    tone: "connection",
  },
  "financial.connection_enabled": {
    category: "Connection",
    title: "Financial connection re-enabled",
    detail: "The preserved connection returned to eligible status.",
    tone: "connection",
  },
  "financial.connection_disconnected": {
    category: "Connection",
    title: "Financial connection disconnected",
    detail: "Arbion retired the connection while preserving durable evidence.",
    tone: "connection",
  },
  "financial.connection_requires_attention": {
    category: "Connection",
    title: "Financial connection needs attention",
    detail: "A credential or provider condition requires owner review.",
    tone: "connection",
  },
  "automation_mandate.created": {
    category: "Autonomy",
    title: "Automation mandate created",
    detail: "A new non-live automation policy was saved.",
    tone: "authority",
  },
  "automation_mandate.autonomy_changed": {
    category: "Autonomy",
    title: "Autonomy policy changed",
    detail: "A new immutable mandate version changed the autonomy boundary.",
    tone: "authority",
  },
  "automation_mandate.schedule_changed": {
    category: "Autonomy",
    title: "Automation schedule changed",
    detail: "A new immutable mandate version changed scheduled evaluation.",
    tone: "authority",
  },
  "automation_mandate.strategy_parameters_changed": {
    category: "Autonomy",
    title: "Strategy parameters changed",
    detail: "A new immutable mandate version changed strategy parameters.",
    tone: "authority",
  },
  "automation_mandate.ai_shadow_parameters_changed": {
    category: "Autonomy",
    title: "AI Shadow parameters changed",
    detail: "A new immutable mandate version changed AI Shadow boundaries.",
    tone: "authority",
  },
  "automation_mandate.paper_options_simulation_attestation_changed": {
    category: "Autonomy",
    title: "Options simulation attestation changed",
    detail:
      "A new immutable mandate version changed the PAPER-only attestation.",
    tone: "authority",
  },
  "strategy_instance.paused": {
    category: "Autonomy",
    title: "Automation paused",
    detail: "Manual and scheduled evaluation stopped for the instance.",
    tone: "authority",
  },
  "strategy_instance.resumed": {
    category: "Autonomy",
    title: "Automation resumed",
    detail: "The exact reviewed non-live mandate resumed evaluation.",
    tone: "authority",
  },
  "strategy_instance.completed": {
    category: "Autonomy",
    title: "Automation finished",
    detail: "The non-live instance completed and retained its evidence.",
    tone: "authority",
  },
  "strategy_instance.shadow_evidence_reviewed": {
    category: "Autonomy",
    title: "Shadow evidence reviewed",
    detail:
      "An MFA-backed review of one exact non-live evidence snapshot was recorded without trading authority.",
    tone: "authority",
  },
  "order_intent.user_reviewed_nonexecuting": {
    category: "Approval",
    title: "Non-executing proposal reviewed",
    detail: "MFA-backed owner review was recorded without broker submission.",
    tone: "authority",
  },
  "authorization.role_changed": {
    category: "Authority",
    title: "Administrative role changed",
    detail: "The account administrative role was changed and audited.",
    tone: "authority",
  },
  "entitlement.changed": {
    category: "Authority",
    title: "Product entitlement changed",
    detail: "The account capability entitlement was changed and audited.",
    tone: "authority",
  },
};

for (const scope of ["automation", "account", "user", "global"] as const) {
  const label = scope === "user" ? "owner-wide" : scope;
  presentations[`${scope}_circuit_breaker.engaged`] = {
    category: "Safety control",
    title: `${label.charAt(0).toUpperCase()}${label.slice(1)} emergency stop engaged`,
    detail:
      "Affected new actions now fail closed; no broker position was changed.",
    tone: "control",
  };
  presentations[`${scope}_circuit_breaker.released`] = {
    category: "Safety control",
    title: `${label.charAt(0).toUpperCase()}${label.slice(1)} emergency stop released`,
    detail: "The audited emergency control was released after review.",
    tone: "control",
  };
}

function presentation(action: string): Presentation {
  return (
    presentations[action] ?? {
      category: "Security",
      title: "Account security event",
      detail: "A bounded account control event was recorded.",
      tone: "authority",
    }
  );
}

function timestamp(value: string) {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.valueOf())) return "Time unavailable";
  return new Intl.DateTimeFormat("en-US", {
    dateStyle: "medium",
    timeStyle: "medium",
    timeZone: "UTC",
  }).format(parsed);
}

export function SecurityActivity({
  initialActivities,
  initialCursor = "",
  available = true,
}: {
  initialActivities: SecurityActivityRecord[];
  initialCursor?: string;
  available?: boolean;
}) {
  const [activities, setActivities] = useState(initialActivities);
  const [cursor, setCursor] = useState(initialCursor);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  async function loadEarlier() {
    if (!cursor || busy) return;
    setBusy(true);
    setMessage("");
    try {
      const response = await fetch(
        `/api/auth/security-activity?limit=20&cursor=${encodeURIComponent(cursor)}`,
        { cache: "no-store" },
      );
      const body = (await response.json().catch(() => null)) as {
        activities?: SecurityActivityRecord[];
        next_cursor?: string;
      } | null;
      if (!response.ok || !body || !Array.isArray(body.activities)) {
        setMessage("Earlier security activity could not be loaded.");
        return;
      }
      setActivities((current) => [
        ...current,
        ...body.activities!.filter(
          (candidate) => !current.some((event) => event.id === candidate.id),
        ),
      ]);
      setCursor(body.next_cursor ?? "");
    } catch {
      setMessage("Earlier security activity could not be loaded.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section
      className="security-activity"
      aria-label="Account security activity"
    >
      <header>
        <div>
          <p className="eyebrow">ACCOUNT SECURITY ACTIVITY</p>
          <h2>Every sensitive control change, preserved.</h2>
          <p>
            Review access, provider connection, autonomy, approval, and
            emergency-control events from one append-only timeline.
          </p>
        </div>
        <span>APPEND-ONLY</span>
      </header>

      {!available ? (
        <div className="security-activity-empty is-unavailable">
          <strong>Security activity could not be verified.</strong>
          <p>Your password and MFA controls remain available.</p>
        </div>
      ) : activities.length === 0 ? (
        <div className="security-activity-empty">
          <strong>No security activity is available yet.</strong>
          <p>New eligible events will appear here automatically.</p>
        </div>
      ) : (
        <ol className="security-activity-list">
          {activities.map((event) => {
            const item = presentation(event.action);
            return (
              <li className={`is-${item.tone}`} key={event.id}>
                <span aria-hidden="true" />
                <div>
                  <header>
                    <strong>{item.title}</strong>
                    <small>{item.category}</small>
                  </header>
                  <p>{item.detail}</p>
                  <time dateTime={event.occurred_at}>
                    {timestamp(event.occurred_at)} UTC
                  </time>
                </div>
              </li>
            );
          })}
        </ol>
      )}

      {cursor && available && (
        <button disabled={busy} onClick={loadEarlier} type="button">
          {busy ? "Loading…" : "Load earlier activity"}
        </button>
      )}
      {message && <p role="status">{message}</p>}
      <footer>
        This view excludes email addresses, network identifiers, metadata
        payloads, provider responses, credentials, holdings, and broker data.
      </footer>
    </section>
  );
}
