"use client";

import { useLayoutEffect, useRef, useState } from "react";
import {
  createCommissioningClient,
  readCommissioningContext,
} from "./commissioning-client";
import {
  parseCommissioningContext,
  type CommissioningContext,
} from "./commissioning-contract";
import { CommissioningWorkspace } from "./commissioning";
import {
  executionLogoutEvent,
  executionSessionChannel,
} from "./session-events";
import styles from "./owner-execution.module.css";

function validated(value: unknown): CommissioningContext {
  try {
    return parseCommissioningContext(value);
  } catch {
    return { available: false };
  }
}

export function CommissioningHost({
  context,
}: {
  context: CommissioningContext;
}) {
  const [original] = useState(() => validated(context));
  const [closed, setClosed] = useState(false);
  const permanentlyClosed = useRef(false),
    generation = useRef(0);
  const [resource, setResource] = useState<{
    client: ReturnType<typeof createCommissioningClient>;
    generation: number;
  } | null>(null);
  const sameContext =
    JSON.stringify(validated(context)) === JSON.stringify(original);
  if (!sameContext && !closed) setClosed(true);
  useLayoutEffect(() => {
    if (!original.available || permanentlyClosed.current) return;
    let alive = true;
    const client = createCommissioningClient(
      original.session_binding,
      original.review,
      () => {
        if (!alive) return;
        permanentlyClosed.current = true;
        setClosed(true);
      },
    );
    setResource({ client, generation: ++generation.current });
    function close() {
      permanentlyClosed.current = true;
      client.invalidate();
      setClosed(true);
    }
    if (!sameContext || document.visibilityState === "hidden") close();
    let pending: AbortController | null = null;
    async function recheck() {
      if (pending || permanentlyClosed.current) return;
      const abort = new AbortController();
      pending = abort;
      const timer = setTimeout(() => abort.abort(), 5_000);
      try {
        const response = await fetch(
          "/api/personal-execution/commissioning/context",
          {
            credentials: "same-origin",
            cache: "no-store",
            redirect: "error",
            headers: { Accept: "application/json" },
            signal: abort.signal,
          },
        );
        const current = await readCommissioningContext(response);
        if (alive && JSON.stringify(current) !== JSON.stringify(original))
          close();
      } catch {
        if (alive) close();
      } finally {
        clearTimeout(timer);
        pending = null;
      }
    }
    function visibility() {
      if (document.visibilityState === "hidden") close();
      else void recheck();
    }
    function focus() {
      void recheck();
    }
    function restored(event: PageTransitionEvent) {
      if (event.persisted) close();
    }
    let channel: BroadcastChannel | null = null;
    try {
      if (typeof BroadcastChannel !== "undefined")
        channel = new BroadcastChannel(executionSessionChannel);
    } catch {
      close();
    }
    if (channel)
      channel.onmessage = (event) => {
        if (event.data === "logout") close();
      };
    window.addEventListener("focus", focus);
    window.addEventListener("pagehide", close);
    window.addEventListener("pageshow", restored);
    window.addEventListener(executionLogoutEvent, close);
    document.addEventListener("visibilitychange", visibility);
    return () => {
      alive = false;
      client.invalidate();
      pending?.abort();
      channel?.close();
      window.removeEventListener("focus", focus);
      window.removeEventListener("pagehide", close);
      window.removeEventListener("pageshow", restored);
      window.removeEventListener(executionLogoutEvent, close);
      document.removeEventListener("visibilitychange", visibility);
    };
  }, [original, sameContext]);
  if (!original.available) return null;
  if (closed || !sameContext)
    return (
      <section className={styles.workspace} role="status">
        <h2>Setup session closed</h2>
        <p>
          Reopen securely and read saved setup and consent. Do not repeat an
          unconfirmed action. Leaving or hiding this page closes its controls.
        </p>
        <a href="/personal-execution">Reopen securely</a>
      </section>
    );
  if (!resource) return <p role="status">Opening authenticated pilot setup…</p>;
  return (
    <CommissioningWorkspace
      key={resource.generation}
      review={original.review}
      client={resource.client}
    />
  );
}
