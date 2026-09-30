"use client";

import { useLayoutEffect, useRef, useState } from "react";
import { createOwnerExecutionClient } from "./client";
import {
  parseOwnerExecutionContext,
  readOwnerExecutionContext,
  type OwnerExecutionContext,
} from "./context";
import { OwnerExecutionWorkspace } from "./owner-execution";
import styles from "./owner-execution.module.css";
import {
  executionLogoutEvent,
  executionSessionChannel,
} from "./session-events";

function validated(value: unknown): OwnerExecutionContext {
  try {
    return parseOwnerExecutionContext(value);
  } catch {
    return { available: false };
  }
}

export function OwnerExecutionHost({
  context,
}: {
  context: OwnerExecutionContext;
}) {
  const [original] = useState(() => validated(context));
  const [closed, setClosed] = useState(false);
  const generation = useRef(0);
  const permanentlyClosed = useRef(false);
  const [resource, setResource] = useState<{
    client: ReturnType<typeof createOwnerExecutionClient>;
    generation: number;
  } | null>(null);
  const sameContext =
    JSON.stringify(validated(context)) === JSON.stringify(original);
  if (!sameContext && !closed) setClosed(true);

  useLayoutEffect(() => {
    if (!original.available || permanentlyClosed.current) return;
    let alive = true;
    const client = createOwnerExecutionClient(original.session_binding, () => {
      if (!alive) return;
      permanentlyClosed.current = true;
      setClosed(true);
    });
    // Resources are scoped to this effect lifetime. React's development replay
    // gets a fresh client and empty workspace, never revives an invalid client.
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
        const response = await fetch("/api/personal-execution/context", {
          credentials: "same-origin",
          cache: "no-store",
          redirect: "error",
          headers: { Accept: "application/json" },
          signal: abort.signal,
        });
        const current = await readOwnerExecutionContext(response);
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

  if (!original.available)
    return (
      <section className={styles.workspace}>
        <h2>Personal execution is not enabled</h2>
        <p>
          No execution account or trading session is connected. This page cannot
          submit orders.
        </p>
      </section>
    );
  if (closed || !sameContext)
    return (
      <section className={styles.workspace} role="status">
        <h2>Execution session closed</h2>
        <p>
          Reopen this page to verify your current sign-in, then read saved order
          status. Do not repeat a send or cancellation. Leaving or hiding this
          page closes its execution controls.
        </p>
        <a href="/personal-execution">Reopen securely</a>
      </section>
    );
  if (!resource)
    return <p role="status">Opening the authenticated workspace…</p>;
  return (
    <OwnerExecutionWorkspace
      key={resource.generation}
      productID={original.product_id}
      accountLabel={original.account_label}
      client={resource.client}
    />
  );
}
