import Link from "next/link";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { SecurityControls } from "./security-controls";

type User = {
  email: string;
  email_verified: boolean;
};

export default async function SecurityPage() {
  const jar = await cookies();
  const response = await fetch(
    `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/auth/me`,
    { headers: { cookie: jar.toString() }, cache: "no-store" },
  );
  if (response.status === 401) redirect("/login");
  if (!response.ok) throw new Error("Unable to load account security");
  const { user } = (await response.json()) as { user: User };

  return (
    <main className="connections-page security-page">
      <Link href="/dashboard">← Dashboard</Link>
      <p className="eyebrow">ACCOUNT SECURITY</p>
      <h1>Protect your access.</h1>
      <p className="security-note">
        Signed in as {user.email}. Email verification is{" "}
        {user.email_verified
          ? "complete"
          : "not yet enabled for private testing"}
        .
      </p>
      <SecurityControls />
    </main>
  );
}
