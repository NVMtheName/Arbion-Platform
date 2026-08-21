"use client";
import Link from "next/link";
import { type FormEvent, useState } from "react";
import type { FinancialAccount, FinancialConnection } from "./page";

export function FinancialManager({
  provider,
  entitled,
  connections: initial,
  accounts,
}: {
  provider: { id: string; label: string; auth_type: string };
  entitled: boolean;
  connections: FinancialConnection[];
  accounts: FinancialAccount[];
}) {
  const [connections, setConnections] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [keyName, setKeyName] = useState("");
  const [privateKey, setPrivateKey] = useState("");
  async function message(response: Response, fallback: string) {
    try {
      const payload = (await response.json()) as {
        error?: { message?: string };
      };
      return payload.error?.message || fallback;
    } catch {
      return fallback;
    }
  }
  async function act(path: string, method = "POST") {
    setBusy(true);
    setError("");
    const r = await fetch(path, { method });
    setBusy(false);
    if (!r.ok) {
      setError(await message(r, "Unable to complete the request."));
      return false;
    }
    return true;
  }
  async function connect() {
    const r = await fetch(`/api/connections/financial/${provider.id}/start`, {
      method: "POST",
    });
    if (!r.ok) {
      setError(await message(r, "Unable to start authorization."));
      return;
    }
    const d = (await r.json()) as { authorization_url: string };
    window.location.assign(d.authorization_url);
  }
  async function connectCoinbase(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    const response = await fetch("/api/connections/financial/coinbase", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ key_name: keyName, private_key: privateKey }),
    });
    setPrivateKey("");
    setBusy(false);
    if (!response.ok) {
      setError(
        await message(
          response,
          "Coinbase did not accept this view-only connection.",
        ),
      );
      return;
    }
    setKeyName("");
    window.location.reload();
  }
  async function change(
    c: FinancialConnection,
    action: "sync" | "enable" | "disable",
  ) {
    if (await act(`/api/connections/financial/${c.id}/${action}`)) {
      if (action !== "sync")
        setConnections((v) =>
          v.map((x) =>
            x.id === c.id
              ? { ...x, status: action === "disable" ? "disabled" : "active" }
              : x,
          ),
        );
      else window.location.reload();
    }
  }
  async function disconnect(c: FinancialConnection) {
    if (await act(`/api/connections/financial/${c.id}`, "DELETE"))
      setConnections((v) => v.filter((x) => x.id !== c.id));
  }
  if (!connections.length && provider.id === "coinbase")
    return (
      <>
        <div className="coinbase-key-guide">
          <strong>Required key restrictions</strong>
          <ul>
            <li>Signature algorithm: ECDSA (ES256)</li>
            <li>Permission: View only</li>
            <li>Trade and Transfer: off</li>
            <li>Recommended IP allowlist: 52.21.127.30</li>
          </ul>
          <a
            href="https://portal.cdp.coinbase.com/"
            target="_blank"
            rel="noreferrer"
          >
            Open Coinbase Developer Platform
          </a>
        </div>
        <form className="coinbase-key-form" onSubmit={connectCoinbase}>
          <label>
            API key name
            <input
              value={keyName}
              onChange={(event) => setKeyName(event.target.value)}
              placeholder="organizations/…/apiKeys/…"
              autoComplete="off"
              spellCheck={false}
              required
            />
          </label>
          <label>
            ECDSA private key
            <textarea
              value={privateKey}
              onChange={(event) => setPrivateKey(event.target.value)}
              placeholder={
                "-----BEGIN EC PRIVATE KEY-----\n…\n-----END EC PRIVATE KEY-----"
              }
              autoComplete="off"
              spellCheck={false}
              required
            />
          </label>
          <p className="credential-assurance">
            Encrypted before storage. Arbion never returns this private key and
            rejects keys that can trade or transfer assets.
          </p>
          <button disabled={!entitled || busy} type="submit">
            {busy ? "Verifying…" : "Connect Coinbase"}
          </button>
        </form>
        {error && <p role="alert">{error}</p>}
      </>
    );
  if (!connections.length)
    return (
      <>
        <p>Not connected</p>
        <button disabled={!entitled || busy} onClick={connect}>
          Connect
        </button>
        {error && <p role="alert">{error}</p>}
      </>
    );
  return (
    <>
      {connections.map((c) => (
        <div className="connection" key={c.id}>
          <strong>{c.status === "active" ? "Connected" : c.status}</strong>
          <p>
            {
              accounts.filter(
                (a) =>
                  a.provider_connection_id === c.id && a.status === "active",
              ).length
            }{" "}
            accounts found
          </p>
          {c.last_synced_at && (
            <p>Last synced: {new Date(c.last_synced_at).toLocaleString()}</p>
          )}
          {accounts
            .filter((a) => a.provider_connection_id === c.id)
            .map((a) => (
              <p key={a.id}>
                <Link href={`/accounts/${a.id}`}>{a.display_name}</Link>
                {a.status !== "active" && ` (${a.status})`}
              </p>
            ))}
          <div className="connection-actions">
            <button
              disabled={busy || c.status === "disabled"}
              onClick={() => change(c, "sync")}
            >
              Sync
            </button>
            <button
              disabled={busy}
              className="secondary"
              onClick={() =>
                change(c, c.status === "disabled" ? "enable" : "disable")
              }
            >
              {c.status === "disabled" ? "Enable" : "Disable"}
            </button>
            <button
              disabled={busy}
              className="danger"
              onClick={() => disconnect(c)}
            >
              Disconnect
            </button>
          </div>
        </div>
      ))}
      {error && <p role="alert">{error}</p>}
    </>
  );
}
