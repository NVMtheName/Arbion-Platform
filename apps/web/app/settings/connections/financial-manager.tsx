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
          "Coinbase did not accept this account connection.",
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
        <p className="connection-card-copy">
          Paste the key name and private key from your Coinbase Developer
          Platform account.
        </p>
        <form className="coinbase-key-form" onSubmit={connectCoinbase}>
          <label>
            Coinbase API key name
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
            Coinbase private key
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
            Encrypted before storage and never displayed again. Transfer-enabled
            keys are rejected.
          </p>
          <button disabled={!entitled || busy} type="submit">
            {busy ? "Verifying…" : "Connect Coinbase"}
          </button>
        </form>
        <details className="connection-details coinbase-key-guide">
          <summary>How to create the right Coinbase key</summary>
          <div>
            <strong>Required key restrictions</strong>
            <ul>
              <li>Signature algorithm: ECDSA (ES256)</li>
              <li>View permission: on</li>
              <li>Trade permission: optional for future execution</li>
              <li>Transfer permission: off (required)</li>
              <li>Recommended IP allowlist: 52.21.127.30</li>
            </ul>
            <a
              href="https://portal.cdp.coinbase.com/"
              target="_blank"
              rel="noreferrer"
            >
              Open Coinbase Developer Platform ↗
            </a>
          </div>
        </details>
        {error && <p role="alert">{error}</p>}
      </>
    );
  if (!connections.length)
    return (
      <>
        <p className="connection-card-copy">
          Authorize Arbion from your provider account. Your provider password
          never passes through Arbion.
        </p>
        <button disabled={!entitled || busy} onClick={connect}>
          Connect {provider.label}
        </button>
        {error && <p role="alert">{error}</p>}
      </>
    );
  return (
    <>
      {connections.map((c) => (
        <div className="connection" key={c.id}>
          <strong>
            {c.status === "active" ? "Account connection active" : c.status}
          </strong>
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
          {provider.id === "coinbase" && (
            <div className="coinbase-permission-state">
              <strong>
                {accounts.some(
                  (account) =>
                    account.provider_connection_id === c.id &&
                    account.capabilities?.provider_trade_authorization ===
                      "SUPPORTED",
                )
                  ? "Coinbase Trade permission granted"
                  : "Coinbase View permission only"}
              </strong>
              <p>
                Transfer permission is rejected. Arbion can request a real
                provider preview, but order submission remains locked behind the
                execution-control milestone.
              </p>
            </div>
          )}
          <div className="connection-actions connection-primary-actions">
            <button
              disabled={busy || c.status === "disabled"}
              onClick={() => change(c, "sync")}
            >
              Refresh accounts
            </button>
          </div>
          <details className="connection-details">
            <summary>Manage connection</summary>
            <div className="connection-actions">
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
          </details>
        </div>
      ))}
      {error && <p role="alert">{error}</p>}
    </>
  );
}
