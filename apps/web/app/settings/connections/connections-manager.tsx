"use client";
import Link from "next/link";
import { FormEvent, useState } from "react";
import type { Connection, Provider } from "./page";

export function ConnectionsManager({
  initialConnections,
  providers,
  entitled,
}: {
  initialConnections: Connection[];
  providers: Provider[];
  entitled: boolean;
}) {
  const [connections, setConnections] = useState(initialConnections);
  const [connecting, setConnecting] = useState<string | null>(null);
  const [replacing, setReplacing] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [models, setModels] = useState<Record<string, Model[]>>({});
  const [selectedConnection, setSelectedConnection] = useState("");
  const [selectedModel, setSelectedModel] = useState("");
  type Model = { id: string; display_name: string; provider: string };
  async function request(path: string, method: string, body?: unknown) {
    setError("");
    const response = await fetch(path, {
      method,
      headers: body ? { "Content-Type": "application/json" } : undefined,
      body: body ? JSON.stringify(body) : undefined,
    });
    if (!response.ok) {
      const value = (await response.json().catch(() => null)) as {
        error?: { message?: string };
      } | null;
      setError(value?.error?.message ?? "Unable to complete the request.");
      return null;
    }
    if (response.status === 204) return {};
    return response.json();
  }
  async function create(e: FormEvent<HTMLFormElement>, provider: string) {
    e.preventDefault();
    const form = e.currentTarget;
    const values = Object.fromEntries(new FormData(form));
    const data = (await request("/api/connections/ai", "POST", {
      provider,
      display_name: values.display_name,
      credential: values.credential,
    })) as { connection: Connection } | null;
    form.reset();
    if (data) {
      setConnections((v) => [...v, data.connection]);
      setConnecting(null);
    }
  }
  async function replace(e: FormEvent<HTMLFormElement>, id: string) {
    e.preventDefault();
    const form = e.currentTarget;
    const credential = new FormData(form).get("credential");
    const data = (await request(`/api/connections/ai/${id}/credential`, "PUT", {
      credential,
    })) as { connection: Connection } | null;
    form.reset();
    if (data) {
      setConnections((v) => v.map((c) => (c.id === id ? data.connection : c)));
      setReplacing(null);
    }
  }
  async function state(c: Connection, action: "enable" | "disable") {
    const data = (await request(
      `/api/connections/ai/${c.id}/${action}`,
      "POST",
    )) as { connection: Connection } | null;
    if (data)
      setConnections((v) =>
        v.map((x) => (x.id === c.id ? data.connection : x)),
      );
  }
  async function remove(c: Connection) {
    const data = await request(`/api/connections/ai/${c.id}`, "DELETE");
    if (data) setConnections((v) => v.filter((x) => x.id !== c.id));
  }
  async function verify(c: Connection) {
    const data = (await request(
      `/api/connections/ai/${c.id}/verify`,
      "POST",
    )) as { connection: Connection } | null;
    if (data) {
      setConnections((items) =>
        items.map((item) => (item.id === c.id ? data.connection : item)),
      );
      await loadModels(data.connection);
    }
  }
  async function loadModels(c: Connection) {
    const data = (await request(
      `/api/connections/ai/${c.id}/models`,
      "GET",
    )) as { models: Model[] } | null;
    if (data) setModels((current) => ({ ...current, [c.id]: data.models }));
  }
  async function saveDefault(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    await request("/api/settings/neural-engine", "PUT", {
      connection_id: selectedConnection,
      model_id: selectedModel,
    });
  }
  return (
    <main className="connections-page">
      <Link href="/dashboard">← Dashboard</Link>
      <p className="eyebrow">SETTINGS</p>
      <h1>Neural Engine Providers</h1>
      <p className="security-note">
        Your API credential is encrypted and stored server-side. Arbion does not
        display the credential again after it is saved.
      </p>
      {!entitled && (
        <p className="unavailable">
          AI-provider connections are unavailable for the current plan.
        </p>
      )}
      {error && (
        <p role="alert" className="form-error">
          {error}
        </p>
      )}
      <section className="provider-list">
        {providers.map((provider) => {
          const items = connections.filter((c) => c.provider === provider.id);
          return (
            <article key={provider.id}>
              <h2>{provider.label}</h2>
              {items.length === 0 ? (
                <>
                  <p>Not connected</p>
                  {entitled && (
                    <button onClick={() => setConnecting(provider.id)}>
                      Connect
                    </button>
                  )}
                </>
              ) : (
                items.map((c) => (
                  <div className="connection" key={c.id}>
                    <strong>{c.display_name}</strong>
                    <p>
                      Status:{" "}
                      {c.status === "pending"
                        ? "Pending verification"
                        : c.status}
                    </p>
                    <p>Credential: {c.credential_hint}</p>
                    {c.last_verified_at && (
                      <p>
                        Verified:{" "}
                        {new Date(c.last_verified_at).toLocaleString()}
                      </p>
                    )}
                    {models[c.id] && (
                      <p>Models available: {models[c.id].length}</p>
                    )}
                    {entitled && (
                      <div className="connection-actions">
                        {c.status !== "disabled" && (
                          <button onClick={() => verify(c)}>
                            {c.status === "active"
                              ? "Reverify"
                              : "Verify Connection"}
                          </button>
                        )}
                        {c.status === "active" && (
                          <button
                            className="secondary"
                            onClick={() => {
                              setSelectedConnection(c.id);
                              void loadModels(c);
                            }}
                          >
                            Configure Neural Engine
                          </button>
                        )}
                        <button
                          className="secondary"
                          onClick={() => setReplacing(c.id)}
                        >
                          Replace Key
                        </button>
                        <button
                          className="secondary"
                          onClick={() =>
                            state(c, c.enabled ? "disable" : "enable")
                          }
                        >
                          {c.enabled ? "Disable" : "Enable"}
                        </button>
                        <button className="danger" onClick={() => remove(c)}>
                          Remove
                        </button>
                      </div>
                    )}
                    {replacing === c.id && (
                      <form onSubmit={(e) => replace(e, c.id)}>
                        <label>
                          New API Key
                          <input
                            name="credential"
                            type="password"
                            required
                            maxLength={4096}
                            autoComplete="off"
                          />
                        </label>
                        <button>Save replacement</button>
                      </form>
                    )}
                  </div>
                ))
              )}
              {connecting === provider.id && (
                <form onSubmit={(e) => create(e, provider.id)}>
                  <p>Provider: {provider.label}</p>
                  <label>
                    Display name
                    <input name="display_name" required maxLength={100} />
                  </label>
                  <label>
                    API Key
                    <input
                      name="credential"
                      type="password"
                      required
                      maxLength={4096}
                      autoComplete="off"
                    />
                  </label>
                  <button>Save Connection</button>
                </form>
              )}
            </article>
          );
        })}
      </section>
      <section className="neural-default">
        <h2>Neural Engine</h2>
        <p>Choose the default provider connection and model for AI features.</p>
        <form onSubmit={saveDefault}>
          <label>
            Connection
            <select
              required
              value={selectedConnection}
              onChange={(event) => {
                const id = event.target.value;
                setSelectedConnection(id);
                setSelectedModel("");
                const connection = connections.find((item) => item.id === id);
                if (connection) void loadModels(connection);
              }}
            >
              <option value="">Select an active connection</option>
              {connections
                .filter((connection) => connection.status === "active")
                .map((connection) => (
                  <option key={connection.id} value={connection.id}>
                    {connection.provider_label} — {connection.display_name}
                  </option>
                ))}
            </select>
          </label>
          <label>
            Model
            <select
              required
              value={selectedModel}
              onChange={(event) => setSelectedModel(event.target.value)}
              disabled={!selectedConnection}
            >
              <option value="">Select a model</option>
              {(models[selectedConnection] ?? []).map((model) => (
                <option key={model.id} value={model.id}>
                  {model.display_name}
                </option>
              ))}
            </select>
          </label>
          <button disabled={!selectedConnection || !selectedModel}>
            Save Default
          </button>
        </form>
      </section>
    </main>
  );
}
