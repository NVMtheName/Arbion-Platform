import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { AppPageHeader } from "../../app-page-header";
import { SecurityControls } from "./security-controls";

type User = {
  email: string;
  email_verified: boolean;
};

type MFAStatus = {
  enabled: boolean;
  recovery_codes_remaining: number;
};

export default async function SecurityPage() {
  const jar = await cookies();
  const base = process.env.API_BASE_URL ?? "http://localhost:8080";
  const options = {
    headers: { cookie: jar.toString() },
    cache: "no-store" as const,
  };
  const [response, mfaResponse] = await Promise.all([
    fetch(`${base}/api/auth/me`, options),
    fetch(`${base}/api/auth/mfa`, options),
  ]);
  if (response.status === 401 || mfaResponse.status === 401) redirect("/login");
  if (!response.ok || !mfaResponse.ok)
    throw new Error("Unable to load account security");
  const { user } = (await response.json()) as { user: User };
  const { mfa } = (await mfaResponse.json()) as { mfa: MFAStatus };

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
      <SecurityControls initialMFAStatus={mfa} />
    </main>
  );
}
