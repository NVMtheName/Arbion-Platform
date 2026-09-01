"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useLayoutEffect, useRef, useState } from "react";

const destinations = [
  { href: "/dashboard", label: "Dashboard", matches: ["/dashboard"] },
  { href: "/accounts", label: "Portfolio", matches: ["/accounts"] },
  { href: "/markets", label: "Markets", matches: ["/markets"] },
  {
    href: "/automations",
    label: "Automations",
    matches: ["/automations"],
  },
  { href: "/activity", label: "Activity", matches: ["/activity"] },
  { href: "/capital", label: "Capital", matches: ["/capital"] },
  {
    href: "/connections",
    label: "Connections",
    matches: ["/connections", "/settings/connections"],
  },
  {
    href: "/settings/security",
    label: "Security",
    matches: ["/settings/security", "/settings/risk"],
  },
] as const;

let retainedNavigationScrollLeft = 0;

const connectionStatuses = new Set([
  "pending",
  "active",
  "expired",
  "revoked",
  "error",
  "disabled",
]);

type ConnectionHealthState =
  | "LOADING"
  | "CURRENT"
  | "RENEWAL_NEEDED"
  | "CONNECTION_FAILED"
  | "NOT_CONNECTED"
  | "UNAVAILABLE";

export type ConnectionNavigationHealth = {
  state: ConnectionHealthState;
  label: string;
  connectionCount: number;
  attentionCount: number;
};

function timestamp(value: unknown) {
  if (typeof value !== "string" || !value) return;
  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? undefined : parsed;
}

export function projectConnectionNavigationHealth(
  connections: unknown,
  observedAt: string,
): ConnectionNavigationHealth {
  const observedTime = timestamp(observedAt);
  if (!Array.isArray(connections) || observedTime === undefined) {
    return {
      state: "UNAVAILABLE",
      label: "Financial connection health unavailable",
      connectionCount: 0,
      attentionCount: 0,
    };
  }
  if (connections.length === 0) {
    return {
      state: "NOT_CONNECTED",
      label: "No financial account connected",
      connectionCount: 0,
      attentionCount: 0,
    };
  }
  const ids = new Set<string>();
  let renewalCount = 0;
  let failedCount = 0;
  for (const candidate of connections) {
    if (!candidate || typeof candidate !== "object") {
      return {
        state: "UNAVAILABLE",
        label: "Financial connection health unavailable",
        connectionCount: connections.length,
        attentionCount: 0,
      };
    }
    const connection = candidate as Record<string, unknown>;
    const id = connection.id;
    const provider = connection.provider;
    const status = connection.status;
    const verifiedAt = timestamp(connection.last_synced_at);
    const expiresAt =
      connection.authorization_expires_at == null
        ? undefined
        : timestamp(connection.authorization_expires_at);
    if (
      typeof id !== "string" ||
      !id ||
      ids.has(id) ||
      typeof provider !== "string" ||
      !provider ||
      typeof status !== "string" ||
      !connectionStatuses.has(status) ||
      (status === "active" && verifiedAt === undefined) ||
      (verifiedAt !== undefined && verifiedAt > observedTime) ||
      (connection.authorization_expires_at != null && expiresAt === undefined)
    ) {
      return {
        state: "UNAVAILABLE",
        label: "Financial connection health unavailable",
        connectionCount: connections.length,
        attentionCount: 0,
      };
    }
    ids.add(id);
    if (status !== "active") {
      failedCount += 1;
      continue;
    }
    if (
      expiresAt !== undefined &&
      expiresAt - observedTime <= 24 * 60 * 60 * 1000
    ) {
      renewalCount += 1;
    }
  }
  if (failedCount > 0) {
    return {
      state: "CONNECTION_FAILED",
      label: `${failedCount} financial ${failedCount === 1 ? "connection needs" : "connections need"} review`,
      connectionCount: connections.length,
      attentionCount: failedCount + renewalCount,
    };
  }
  if (renewalCount > 0) {
    return {
      state: "RENEWAL_NEEDED",
      label: `${renewalCount} financial connection ${renewalCount === 1 ? "renewal is" : "renewals are"} needed now or within 24 hours`,
      connectionCount: connections.length,
      attentionCount: renewalCount,
    };
  }
  return {
    state: "CURRENT",
    label: `${connections.length} financial ${connections.length === 1 ? "connection is" : "connections are"} current`,
    connectionCount: connections.length,
    attentionCount: 0,
  };
}

function ConnectionNavigationHealthSignal() {
  const [health, setHealth] = useState<ConnectionNavigationHealth>({
    state: "LOADING",
    label: "Checking financial connection health",
    connectionCount: 0,
    attentionCount: 0,
  });
  useEffect(() => {
    const controller = new AbortController();
    async function load() {
      try {
        const response = await fetch("/api/connections/financial", {
          cache: "no-store",
          signal: controller.signal,
        });
        const body = (await response.json().catch(() => undefined)) as
          | { connections?: unknown }
          | undefined;
        if (!response.ok || !body) throw new Error("connection health");
        setHealth(
          projectConnectionNavigationHealth(
            body.connections,
            new Date().toISOString(),
          ),
        );
      } catch {
        if (controller.signal.aborted) return;
        setHealth({
          state: "UNAVAILABLE",
          label: "Financial connection health unavailable",
          connectionCount: 0,
          attentionCount: 0,
        });
      }
    }
    void load();
    return () => controller.abort();
  }, []);
  return (
    <span
      className={`connection-navigation-health is-${health.state.toLowerCase().replaceAll("_", "-")}`}
      role="status"
      aria-label={health.label}
      title={health.label}
    />
  );
}

function isCurrent(pathname: string, matches: readonly string[]) {
  return matches.some(
    (match) => pathname === match || pathname.startsWith(`${match}/`),
  );
}

export function AppNavigation({ className = "" }: { className?: string }) {
  const pathname = usePathname();
  const navigationRef = useRef<HTMLElement>(null);
  const retainScrollPosition = () => {
    if (navigationRef.current) {
      retainedNavigationScrollLeft = navigationRef.current.scrollLeft;
    }
  };
  useLayoutEffect(() => {
    const navigation = navigationRef.current;
    if (!navigation) return;
    navigation.scrollLeft = Math.min(
      retainedNavigationScrollLeft,
      Math.max(0, navigation.scrollWidth - navigation.clientWidth),
    );
  }, []);
  return (
    <nav
      ref={navigationRef}
      className={["app-navigation", className].filter(Boolean).join(" ")}
      aria-label="Application navigation"
      onScroll={retainScrollPosition}
    >
      {destinations.map((destination) => {
        const current = isCurrent(pathname, destination.matches);
        const isConnections = destination.href === "/connections";
        return (
          <Link
            className={
              [current ? "is-current" : "", isConnections ? "has-health" : ""]
                .filter(Boolean)
                .join(" ") || undefined
            }
            href={destination.href}
            aria-label={destination.label}
            aria-current={current ? "page" : undefined}
            key={destination.href}
            onClick={retainScrollPosition}
          >
            {destination.label}
            {isConnections ? <ConnectionNavigationHealthSignal /> : null}
          </Link>
        );
      })}
    </nav>
  );
}
