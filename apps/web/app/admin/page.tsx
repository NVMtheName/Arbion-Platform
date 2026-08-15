import Link from "next/link";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
type User = { id: string; email: string; role: string; entitlement: string };
export default async function Admin() {
  const c = await cookies();
  const base = process.env.API_BASE_URL ?? "http://localhost:8080";
  const me = await fetch(`${base}/api/admin/me`, {
    headers: { cookie: c.toString() },
    cache: "no-store",
  });
  if (!me.ok) redirect("/dashboard");
  const { user } = (await me.json()) as {
    user: { role: string; entitlement: string };
  };
  const response = await fetch(`${base}/api/admin/users`, {
    headers: { cookie: c.toString() },
    cache: "no-store",
  });
  if (!response.ok) redirect("/dashboard");
  const { users } = (await response.json()) as { users: User[] };
  return (
    <main className="dashboard">
      <Link href="/dashboard">← Dashboard</Link>
      <p className="eyebrow">ARBION ADMIN</p>
      <h1>Arbion Admin</h1>
      <p>System role: {user.role}</p>
      <p>entitlement: {user.entitlement}</p>
      <h2>Users</h2>
      <div className="admin-table">
        {users.map((u) => (
          <Link key={u.id} href={`/admin/users/${u.id}`}>
            <span>{u.email}</span>
            <span>{u.role}</span>
            <span>{u.entitlement}</span>
          </Link>
        ))}
      </div>
    </main>
  );
}
