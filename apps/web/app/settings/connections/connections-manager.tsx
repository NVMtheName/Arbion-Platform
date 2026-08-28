"use client";
import { FormEvent, useCallback, useEffect, useState } from "react";

import type { Connection, NeuralPreference, Provider } from "./page";

type Model = { id: string; display_name: string; provider: string };

function countLabel(count: number, singular: string, plural: string) {
  return `${count} ${count === 1 ? singular : plural}`;
}

function preserveContinuity(current: Connection, updated: Connection) {
  return {
    ...updated,
    runtime_protected: current.runtime_protected,
    removal_protected: current.removal_protected,
    protected_mandate_count: current.protected_mandate_count,
    active_strategy_count: current.active_strategy_count,
    retained_automation_count: current.retained_automation_count,
    default_model_selected: current.default_model_selected,
  };
}

function AIConnectionContinuity({ connection }: { connection: Connection }) {
  const runtimeSummary = [
    countLabel(
      connection.active_strategy_count,
      "active or paused engine",
      "active or paused engines",
    ),
    countLabel(
      connection.protected_mandate_count,
      "ready or paused mandate",
      "ready or paused mandates",
    ),
  ].join(" · ");
  const retainedSummary = [
    connection.default_model_selected ? "Default model" : null,
    connection.retained_automation_count > 0
      ? countLabel(
          connection.retained_automation_count,
          "saved automation",
          "saved automations",
        )
      : null,
  ]
    .filter(Boolean)
    .join(" · ");

  return (
    <section
      className={`connection-continuity ${
        connection.runtime_protected
          ? "is-protected"
          : connection.removal_protected
            ? "is-retained"
            : "is-clear"
      }`}
      id={`ai-connection-continuity-${connection.id}`}
      aria-label={`${connection.display_name} connection continuity`}
    >
      <strong>
        {connection.runtime_protected
          ? "AI engine continuity protected"
          : connection.removal_protected
            ? "Configuration identity retained"
            : "No protected automation dependency"}
      </strong>
      <p>
        {connection.runtime_protected
          ? `${runtimeSummary}. Arbion blocks disabling this connection while those controls depend on it.`
          : connection.removal_protected
            ? `${retainedSummary}. No active engine uses this connection, but removal stays blocked so saved identity and evidence remain intact.`
            : "No active engine, reviewed mandate, default model, or saved automation currently retains this connection."}
      </p>
      <small>
        {connection.runtime_protected
          ? "Verified key rotation preserves the same connection identity."
          : "The server rechecks dependencies again before every state change."}
      </small>
    </section>
  );
}

export function ConnectionsManager({
  initialConnections,
  initialPreference,
  providers,
  entitled,
}: {
  initialConnections: Connection[];
  initialPreference: NeuralPreference | null;
  providers: Provider[];
  entitled: boolean;
}) {
  const [connections, setConnections] = useState(initialConnections);
  const [connecting, setConnecting] = useState<string | null>(null);
  const [replacing, setReplacing] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [models, setModels] = useState<Record<string, Model[]>>({});
  const [selectedConnection, setSelectedConnection] = useState(
    initialPreference?.connection_id ?? "",
  );
  const [selectedModel, setSelectedModel] = useState(
    initialPreference?.model_id ?? "",
  );
  const [savedPreference, setSavedPreference] = useState(initialPreference);
  const [saveMessage, setSaveMessage] = useState("");
  const request = useCallback(async function request(
    path: string,
    method: string,
    body?: unknown,
  ) {
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
  }, []);
  async function create(e: FormEvent<HTMLFormElement>, provider: Provider) {
    e.preventDefault();
    const form = e.currentTarget;
    const values = Object.fromEntries(new FormData(form));
    const data = (await request("/api/connections/ai", "POST", {
      provider: provider.id,
      display_name: values.display_name || provider.label,
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
      setConnections((v) =>
        v.map((c) =>
          c.id === id ? preserveContinuity(c, data.connection) : c,
        ),
      );
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
        v.map((x) =>
          x.id === c.id ? preserveContinuity(x, data.connection) : x,
        ),
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
        items.map((item) =>
          item.id === c.id ? preserveContinuity(item, data.connection) : item,
        ),
      );
      setSelectedConnection(data.connection.id);
      setSelectedModel("");
      await loadModels(data.connection);
    }
  }
  const loadModels = useCallback(
    async function loadModels(c: Connection) {
      const data = (await request(
        `/api/connections/ai/${c.id}/models`,
        "GET",
      )) as { models: Model[] } | null;
      if (data) setModels((current) => ({ ...current, [c.id]: data.models }));
    },
    [request],
  );
  useEffect(() => {
    if (!selectedConnection || models[selectedConnection]) return;
    const connection = connections.find(
      (item) => item.id === selectedConnection && item.status === "active",
    );
    if (connection) void loadModels(connection);
  }, [connections, loadModels, models, selectedConnection]);
  async function saveDefault(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setSaveMessage("");
    const data = await request("/api/settings/neural-engine", "PUT", {
      connection_id: selectedConnection,
      model_id: selectedModel,
    });
    if (data) {
      setSavedPreference({
        connection_id: selectedConnection,
        model_id: selectedModel,
      });
      setConnections((items) =>
        items.map((item) => {
          const selected = item.id === selectedConnection;
          return {
            ...item,
            default_model_selected: selected,
            removal_protected: selected || item.retained_automation_count > 0,
          };
        }),
      );
      setSaveMessage(
        "Default model saved. Arbion will use it for new AI-assisted work.",
      );
    }
  }
  return (
    <section
      className="connection-hub-section"
      id="ai-providers"
      aria-labelledby="ai-provider-title"
    >
      <header className="connection-section-heading">
        <div>
          <p className="connection-step-label">STEP 2</p>
          <h2 id="ai-provider-title">Connect an AI provider</h2>
        </div>
        <p>
          Add the API key from OpenAI, Anthropic, or Google. The key is
          encrypted and is never shown again after it is saved.
        </p>
      </header>
      {!entitled && (
        <p className="connection-quiet-state">
          AI-provider connections are unavailable for the current plan.
        </p>
      )}
      {error && (
        <p role="alert" className="form-error">
          {error}
        </p>
      )}
      <div className="ai-provider-grid">
        {providers.map((provider) => {
          const items = connections.filter((c) => c.provider === provider.id);
          return (
            <article className="ai-provider-card" key={provider.id}>
              <header>
                <span className={`provider-mark provider-${provider.id}`}>
                  {provider.label.slice(0, 1)}
                </span>
                <div>
                  <h3>{provider.label}</h3>
                  <p>Bring your own API key</p>
                </div>
              </header>
              {items.length === 0 ? (
                <>
                  <p className="connection-card-state">Not connected</p>
                  {entitled && (
                    <button onClick={() => setConnecting(provider.id)}>
                      Add API key
                    </button>
                  )}
                </>
              ) : (
                items.map((c) => (
                  <div className="connection" key={c.id}>
                    <strong>{c.display_name}</strong>
                    <p>
                      {c.status === "pending"
                        ? "Waiting for verification"
                        : c.status === "active"
                          ? "Connected and verified"
                          : "Connection disabled"}
                    </p>
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
                            onClick={() => {
                              setSelectedConnection(c.id);
                              void loadModels(c);
                            }}
                          >
                            Choose model
                          </button>
                        )}
                      </div>
                    )}
                    <AIConnectionContinuity connection={c} />
                    <details className="connection-details">
                      <summary>Manage connection</summary>
                      <div>
                        <p>Stored credential: {c.credential_hint}</p>
                        {c.last_verified_at && (
                          <p>
                            Last verified:{" "}
                            {new Date(c.last_verified_at).toLocaleString()}
                          </p>
                        )}
                        <div className="connection-actions">
                          <button
                            className="secondary"
                            onClick={() => setReplacing(c.id)}
                          >
                            Replace key
                          </button>
                          <button
                            className="secondary"
                            disabled={c.enabled && c.runtime_protected}
                            aria-describedby={`ai-connection-continuity-${c.id}`}
                            onClick={() =>
                              state(c, c.enabled ? "disable" : "enable")
                            }
                          >
                            {c.enabled ? "Disable" : "Enable"}
                          </button>
                          <button
                            className="danger"
                            disabled={c.removal_protected}
                            aria-describedby={`ai-connection-continuity-${c.id}`}
                            onClick={() => remove(c)}
                          >
                            Remove
                          </button>
                        </div>
                        {replacing === c.id && (
                          <form onSubmit={(e) => replace(e, c.id)}>
                            <p className="connection-card-state">
                              {c.status === "active"
                                ? "Arbion encrypts and verifies the candidate while your current key stays active. Only a successful candidate replaces it."
                                : "The replacement will be stored as pending and must be verified before Arbion can use it."}
                            </p>
                            <label>
                              New API key
                              <input
                                name="credential"
                                type="password"
                                required
                                maxLength={4096}
                                autoComplete="off"
                              />
                            </label>
                            <button>
                              {c.status === "active"
                                ? "Verify and rotate key"
                                : "Store replacement for verification"}
                            </button>
                          </form>
                        )}
                      </div>
                    </details>
                  </div>
                ))
              )}
              {connecting === provider.id && (
                <form onSubmit={(event) => create(event, provider)}>
                  <label>
                    Connection name <span>(optional)</span>
                    <input
                      name="display_name"
                      maxLength={100}
                      placeholder={`My ${provider.label} connection`}
                    />
                  </label>
                  <label>
                    API key
                    <input
                      name="credential"
                      type="password"
                      required
                      maxLength={4096}
                      autoComplete="off"
                    />
                  </label>
                  <button>Save API key</button>
                </form>
              )}
            </article>
          );
        })}
      </div>
      <section className="model-choice" id="model-choice">
        <div>
          <p className="connection-step-label">STEP 3</p>
          <h2>Choose your default model</h2>
          <p>
            You can change this later. Strategies may use a different verified
            model when you explicitly choose one.
          </p>
        </div>
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
              <option value="">Choose an AI provider</option>
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
              <option value="">Choose a verified model</option>
              {(models[selectedConnection] ?? []).map((model) => (
                <option key={model.id} value={model.id}>
                  {model.display_name}
                </option>
              ))}
            </select>
          </label>
          <button disabled={!selectedConnection || !selectedModel}>
            Save default model
          </button>
          {saveMessage && (
            <p className="form-success" role="status">
              {saveMessage}
            </p>
          )}
          {!saveMessage && savedPreference && (
            <p className="connection-saved-model">
              Current default: {savedPreference.model_id}
            </p>
          )}
        </form>
      </section>
    </section>
  );
}
