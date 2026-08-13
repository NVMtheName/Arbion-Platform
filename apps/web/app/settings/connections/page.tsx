import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { ConnectionsManager } from "./connections-manager";

export type Connection = {
  id: string;
  provider: string;
  provider_label: string;
  display_name: string;
  status: string;
  enabled: boolean;
  credential_hint: string;
  last_verified_at?: string;
};
export type Provider = {
  id: string;
  label: string;
  credential_types: string[];
  capabilities: string[];
};
export default async function ConnectionsPage() {
  const jar = await cookies();
  const response = await fetch(
    `${process.env.API_BASE_URL ?? "http://localhost:8080"}/api/connections/ai`,
    { headers: { cookie: jar.toString() }, cache: "no-store" },
  );
  if (response.status === 401) redirect("/login");
  if (!response.ok) throw new Error("Unable to load provider connections");
  const data = (await response.json()) as {
    connections: Connection[];
    providers: Provider[];
    can_use_neural_engine: boolean;
  };
  return (
    <ConnectionsManager
      initialConnections={data.connections}
      providers={data.providers}
      entitled={data.can_use_neural_engine}
    />
  );
}
