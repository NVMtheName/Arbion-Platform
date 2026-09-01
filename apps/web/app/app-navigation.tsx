"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type MouseEvent as ReactMouseEvent,
} from "react";

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

const navigationScrollStorageKey = "arbion-navigation-scroll-left";

type NavigationScrollEvidence = {
  retainedScrollLeft: number;
  scrollWidth: number;
  viewportWidth: number;
  activeStart: number;
  activeWidth: number;
};

export function resolveNavigationScrollLeft({
  retainedScrollLeft,
  scrollWidth,
  viewportWidth,
  activeStart,
  activeWidth,
}: NavigationScrollEvidence) {
  const values = [
    retainedScrollLeft,
    scrollWidth,
    viewportWidth,
    activeStart,
    activeWidth,
  ];
  if (values.some((value) => !Number.isFinite(value) || value < 0)) return 0;
  const maximumScrollLeft = Math.max(0, scrollWidth - viewportWidth);
  const retained = Math.min(retainedScrollLeft, maximumScrollLeft);
  if (viewportWidth === 0 || activeWidth === 0) return retained;
  const activeEnd = activeStart + activeWidth;
  if (activeWidth >= viewportWidth || activeStart < retained) {
    return Math.min(activeStart, maximumScrollLeft);
  }
  if (activeEnd > retained + viewportWidth) {
    return Math.min(activeEnd - viewportWidth, maximumScrollLeft);
  }
  return retained;
}

function readNavigationScrollLeft() {
  try {
    const value = Number(
      window.sessionStorage.getItem(navigationScrollStorageKey),
    );
    return Number.isFinite(value) && value >= 0 ? value : 0;
  } catch {
    return 0;
  }
}

function writeNavigationScrollLeft(value: number) {
  try {
    window.sessionStorage.setItem(navigationScrollStorageKey, String(value));
  } catch {
    // Navigation remains usable when browser storage is unavailable.
  }
}

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
  const [pendingNavigation, setPendingNavigation] = useState<{
    href: string;
    fromPathname: string;
  }>();
  const pendingHref =
    pendingNavigation?.fromPathname === pathname
      ? pendingNavigation.href
      : undefined;
  useEffect(() => {
    if (!pendingHref) return;
    const timeout = window.setTimeout(
      () => setPendingNavigation(undefined),
      10_000,
    );
    return () => window.clearTimeout(timeout);
  }, [pendingHref]);
  const retainScrollPosition = () => {
    if (navigationRef.current) {
      writeNavigationScrollLeft(navigationRef.current.scrollLeft);
    }
  };
  useLayoutEffect(() => {
    const navigation = navigationRef.current;
    if (!navigation) return;
    const activeDestination = navigation.querySelector<HTMLElement>(
      'a[aria-current="page"]',
    );
    const scrollLeft = resolveNavigationScrollLeft({
      retainedScrollLeft: readNavigationScrollLeft(),
      scrollWidth: navigation.scrollWidth,
      viewportWidth: navigation.clientWidth,
      activeStart: activeDestination?.offsetLeft ?? 0,
      activeWidth: activeDestination?.offsetWidth ?? 0,
    });
    navigation.scrollLeft = scrollLeft;
    writeNavigationScrollLeft(scrollLeft);
  }, [pathname]);
  const beginNavigation = (
    event: ReactMouseEvent<HTMLAnchorElement>,
    href: string,
    current: boolean,
  ) => {
    retainScrollPosition();
    if (
      current ||
      event.defaultPrevented ||
      event.button !== 0 ||
      event.metaKey ||
      event.ctrlKey ||
      event.shiftKey ||
      event.altKey
    ) {
      return;
    }
    setPendingNavigation({ href, fromPathname: pathname });
  };
  const pendingDestination = destinations.find(
    (destination) => destination.href === pendingHref,
  );
  return (
    <nav
      ref={navigationRef}
      className={[
        "app-navigation",
        pendingDestination ? "is-switching" : "",
        className,
      ]
        .filter(Boolean)
        .join(" ")}
      aria-label="Application navigation"
      aria-busy={pendingDestination ? "true" : undefined}
      onScroll={retainScrollPosition}
    >
      {destinations.map((destination) => {
        const current = isCurrent(pathname, destination.matches);
        const isConnections = destination.href === "/connections";
        const pending = destination.href === pendingHref;
        return (
          <Link
            className={
              [
                current ? "is-current" : "",
                pending ? "is-pending" : "",
                isConnections ? "has-health" : "",
              ]
                .filter(Boolean)
                .join(" ") || undefined
            }
            href={destination.href}
            aria-label={destination.label}
            aria-current={current ? "page" : undefined}
            key={destination.href}
            onClick={(event) =>
              beginNavigation(event, destination.href, current)
            }
          >
            {destination.label}
            {isConnections ? <ConnectionNavigationHealthSignal /> : null}
          </Link>
        );
      })}
      <span
        className="app-navigation-status"
        role="status"
        aria-live="polite"
        aria-atomic="true"
        aria-label={
          pendingDestination ? `Opening ${pendingDestination.label}` : undefined
        }
      >
        {pendingDestination ? `Opening ${pendingDestination.label}` : ""}
      </span>
    </nav>
  );
}
