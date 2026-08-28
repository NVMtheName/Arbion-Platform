import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { AppPageHeader } from "../../app-page-header";
import {
  SecurityActivity,
  type SecurityActivityRecord,
} from "./security-activity";
import { SecurityControls, type SessionInventory } from "./security-controls";

type User = {
  email: string;
  email_verified: boolean;
};

type MFAStatus = {
  enabled: boolean;
  recovery_codes_remaining: number;
};

function validSessionInventory(value: unknown): value is SessionInventory {
  if (!value || typeof value !== "object") return false;
  const candidate = value as Partial<SessionInventory>;
  const current = candidate.current;
  if (
    !Number.isInteger(candidate.active_count) ||
    !Number.isInteger(candidate.other_count) ||
    Number(candidate.active_count) < 1 ||
    Number(candidate.active_count) > 100 ||
    Number(candidate.other_count) !== Number(candidate.active_count) - 1 ||
    !current ||
    typeof current.created_at !== "string" ||
    typeof current.expires_at !== "string"
  )
    return false;
  const created = Date.parse(current.created_at);
  const expires = Date.parse(current.expires_at);
  return (
    Number.isFinite(created) && Number.isFinite(expires) && expires > created
  );
}

export default async function SecurityPage() {
  const jar = await cookies();
  const base = process.env.API_BASE_URL ?? "http://localhost:8080";
  const options = {
    headers: { cookie: jar.toString() },
    cache: "no-store" as const,
  };
  const [response, mfaResponse, activityResponse, sessionsResponse] =
    await Promise.all([
      fetch(`${base}/api/auth/me`, options),
      fetch(`${base}/api/auth/mfa`, options),
      fetch(`${base}/api/auth/security-activity?limit=20`, options),
      fetch(`${base}/api/auth/sessions`, options),
    ]);
  if (
    response.status === 401 ||
    mfaResponse.status === 401 ||
    activityResponse.status === 401 ||
    sessionsResponse.status === 401
  )
    redirect("/login");
  if (!response.ok || !mfaResponse.ok)
    throw new Error("Unable to load account security");
  const { user } = (await response.json()) as { user: User };
  const { mfa } = (await mfaResponse.json()) as { mfa: MFAStatus };
  const activity = activityResponse.ok
    ? ((await activityResponse.json()) as {
        activities?: SecurityActivityRecord[];
        next_cursor?: string;
      })
    : {};
  const sessionPayload = sessionsResponse.ok
    ? ((await sessionsResponse.json().catch(() => null)) as {
        session_inventory?: unknown;
      } | null)
    : {};
  const sessionInventory = validSessionInventory(
    sessionPayload?.session_inventory,
  )
    ? sessionPayload.session_inventory
    : null;

  return (
    <main className="connections-page security-page">
      <AppPageHeader />
      <p className="eyebrow">ACCOUNT SECURITY</p>
      <h1>Protect your access.</h1>
      <p className="security-note">
        Signed in as {user.email}. Email verification is{" "}
        {user.email_verified
          ? "complete"
          : "not yet enabled for private testing"}
        .
      </p>
      <SecurityControls
        initialMFAStatus={mfa}
        initialSessionInventory={sessionInventory}
        sessionInventoryAvailable={
          sessionsResponse.ok && sessionInventory !== null
        }
      />
      <SecurityActivity
        available={activityResponse.ok}
        initialActivities={
          Array.isArray(activity.activities) ? activity.activities : []
        }
        initialCursor={String(activity.next_cursor ?? "")}
      />
    </main>
  );
}
