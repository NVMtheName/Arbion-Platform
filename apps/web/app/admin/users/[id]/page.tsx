import { cookies } from "next/headers";
import { redirect } from "next/navigation";

import { AppPageHeader } from "../../../app-page-header";
export default async function UserDetail({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  const c = await cookies();
  const response = await fetch(
    `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/admin/users/${id}`,
    { headers: { cookie: c.toString() }, cache: "no-store" },
  );
  if (!response.ok) redirect("/admin");
  const { user } = (await response.json()) as {
    user: {
      email: string;
      role: string;
      entitlement: string;
      billing_required: boolean;
    };
  };
  return (
    <main className="dashboard command-content-continuity">
      <AppPageHeader backHref="/admin" backLabel="Admin" />
      <p className="eyebrow">ARBION ADMIN</p>
      <h1>User detail</h1>
      <dl>
        <dt>email</dt>
        <dd>{user.email}</dd>
        <dt>role</dt>
        <dd>{user.role}</dd>
        <dt>entitlement</dt>
        <dd>{user.entitlement}</dd>
        <dt>Billing required</dt>
        <dd>{String(user.billing_required)}</dd>
      </dl>
      <p>
        Superadmins can update this account through the explicit role and
        entitlement admin API endpoints.
      </p>
    </main>
  );
}
