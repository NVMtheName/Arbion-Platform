"use client";
import Link from "next/link";
import { type FormEvent, useEffect, useRef, useState } from "react";
import type { FinancialAccount, FinancialConnection } from "./page";

function coinbaseKeyBundle(value: string) {
  const parsed = JSON.parse(value) as unknown;
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed))
    return null;
  const record = parsed as Record<string, unknown>;
  const name = record.name ?? record.keyName ?? record.key_name;
  const privateKey =
    record.privateKey ?? record.private_key ?? record.apiPrivateKey;
  if (typeof name !== "string" || typeof privateKey !== "string") return null;
  return { name, privateKey };
}

function coinbaseCredentialProblem(name: string, privateKey: string) {
  const normalizedName = name.trim().replace(/^"|"$/g, "");
  const normalizedPrivateKey = privateKey.trim().replace(/^"|"$/g, "");
  if (
    !/^organizations\/[A-Za-z0-9_-]+\/apiKeys\/[A-Za-z0-9_-]+$/.test(
      normalizedName,
    )
  ) {
    return "That is not a current Coinbase CDP key name. It must start with organizations/ and contain /apiKeys/.";
  }
  if (!/^-----BEGIN (?:EC )?PRIVATE KEY-----/.test(normalizedPrivateKey)) {
    return "That is a one-line API secret, not the ECDSA private key Coinbase App requires. Create a new Secret API Key, open Advanced Settings, and choose ECDSA instead of Ed25519.";
  }
  return "";
}

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
  const [keyJSON, setKeyJSON] = useState("");
  const [keyName, setKeyName] = useState("");
  const [privateKey, setPrivateKey] = useState("");
  const coinbaseErrorRef = useRef<HTMLParagraphElement>(null);
  useEffect(() => {
    if (error && provider.id === "coinbase" && !connections.length) {
      coinbaseErrorRef.current?.focus();
    }
  }, [connections.length, error, provider.id]);
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
    setBusy(true);
    setError("");
    const r = await fetch(`/api/connections/financial/${provider.id}/start`, {
      method: "POST",
    });
    if (!r.ok) {
      setError(await message(r, "Unable to start authorization."));
      setBusy(false);
      return;
    }
    const d = (await r.json()) as { authorization_url: string };
    window.location.assign(d.authorization_url);
  }
  async function connectCoinbase(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    let submittedName = keyName;
    let submittedPrivateKey = privateKey;
    if (keyJSON.trim()) {
      try {
        const parsed = coinbaseKeyBundle(keyJSON);
        if (!parsed) throw new Error("invalid Coinbase key JSON");
        submittedName = parsed.name;
        submittedPrivateKey = parsed.privateKey;
      } catch {
        setError(
          "That JSON does not contain Coinbase’s name and privateKey values. Paste the complete downloaded key JSON, or enter both values separately.",
        );
        return;
      }
    }
    if (!submittedName.trim() || !submittedPrivateKey.trim()) {
      setError(
        "Enter both the full Coinbase key name and its ECDSA private key. Downloaded JSON is optional.",
      );
      return;
    }
    const credentialProblem = coinbaseCredentialProblem(
      submittedName,
      submittedPrivateKey,
    );
    if (credentialProblem) {
      setError(credentialProblem);
      return;
    }
    setBusy(true);
    setError("");
    const body = JSON.stringify({
      key_name: submittedName,
      private_key: submittedPrivateKey,
    });
    setKeyJSON("");
    setPrivateKey("");
    const response = await fetch("/api/connections/financial/coinbase", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body,
    });
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
          Enter the two ECDSA key values from Coinbase. You do not need a JSON
          file.
        </p>
        <form className="coinbase-key-form" onSubmit={connectCoinbase}>
          <label>
            Coinbase key name
            <input
              value={keyName}
              onChange={(event) => {
                setKeyName(event.target.value);
                setError("");
              }}
              placeholder="organizations/…/apiKeys/…"
              autoComplete="off"
              spellCheck={false}
            />
          </label>
          <label>
            ECDSA private key
            <textarea
              value={privateKey}
              onChange={(event) => {
                setPrivateKey(event.target.value);
                setError("");
              }}
              placeholder={
                "-----BEGIN EC PRIVATE KEY-----\\n…\\n-----END EC PRIVATE KEY-----"
              }
              autoComplete="off"
              spellCheck={false}
            />
          </label>
          <p className="credential-assurance coinbase-key-compatibility">
            Compatible keys start with <code>organizations/</code>. Their secret
            starts with <code>-----BEGIN EC PRIVATE KEY-----</code>. A short,
            one-line secret is the wrong key type.
          </p>
          <details className="connection-details coinbase-json-entry">
            <summary>Have downloaded key JSON? Paste it instead</summary>
            <div>
              <label>
                Downloaded Coinbase key JSON
                <textarea
                  value={keyJSON}
                  onChange={(event) => {
                    setKeyJSON(event.target.value);
                    setError("");
                  }}
                  placeholder={
                    '{"name":"organizations/…/apiKeys/…","privateKey":"-----BEGIN EC PRIVATE KEY-----\\n…"}'
                  }
                  autoComplete="off"
                  spellCheck={false}
                />
              </label>
            </div>
          </details>
          <p className="credential-assurance">
            Encrypted before storage and never displayed again. Transfer-enabled
            keys are rejected.
          </p>
          {error && (
            <p
              className="form-error coinbase-connection-error"
              id="coinbase-connection-error"
              ref={coinbaseErrorRef}
              role="alert"
              tabIndex={-1}
            >
              <strong>Connection not completed.</strong> {error}
            </p>
          )}
          <button
            aria-busy={busy}
            aria-describedby={error ? "coinbase-connection-error" : undefined}
            disabled={!entitled || busy}
            type="submit"
          >
            {busy ? "Verifying…" : "Connect Coinbase"}
          </button>
        </form>
        <details className="connection-details coinbase-key-guide">
          <summary>How to create the right Coinbase key</summary>
          <div>
            <strong>Required key restrictions</strong>
            <ul>
              <li>Use a new Secret API Key, not a legacy key</li>
              <li>Open API restrictions and Advanced Settings</li>
              <li>Signature algorithm: ECDSA (ES256)</li>
              <li>View permission: on</li>
              <li>Trade permission: optional for future execution</li>
              <li>Transfer permission: off (required)</li>
              <li>Recommended IP allowlist: 52.21.127.30</li>
            </ul>
            <a
              href="https://docs.cdp.coinbase.com/coinbase-app/authentication-authorization/api-key-authentication"
              target="_blank"
              rel="noreferrer"
            >
              Read Coinbase’s official key guide ↗
            </a>
          </div>
        </details>
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
            {c.status === "active"
              ? "Account connection active"
              : c.status === "expired"
                ? "Schwab reconnect required"
                : c.status}
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
          {provider.id === "schwab" && c.status === "active" && (
            <div className="schwab-authorization-state">
              <strong>Weekly Schwab authorization active</strong>
              {c.authorization_expires_at && (
                <p>
                  Renew by{" "}
                  <time
                    dateTime={c.authorization_expires_at}
                    suppressHydrationWarning
                  >
                    {new Date(c.authorization_expires_at).toLocaleString()}
                  </time>
                  . You can renew early without changing your linked account.
                </p>
              )}
            </div>
          )}
          {provider.id === "schwab" &&
            ["expired", "error"].includes(c.status) && (
              <div className="schwab-reconnect-state" role="status">
                <strong>Your brokerage account is still safely linked.</strong>
                <p>
                  Schwab requires you to approve API access again each week.
                  Reconnecting replaces only the expired authorization; it does
                  not change your Arbion account, strategy, or risk settings.
                </p>
                <button disabled={busy || !entitled} onClick={connect}>
                  {busy ? "Opening Schwab…" : "Reconnect Schwab"}
                </button>
              </div>
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
              disabled={busy || c.status !== "active"}
              onClick={() => change(c, "sync")}
            >
              Refresh accounts
            </button>
            {provider.id === "schwab" && c.status === "active" && (
              <button disabled={busy || !entitled} onClick={connect}>
                Renew Schwab access
              </button>
            )}
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
