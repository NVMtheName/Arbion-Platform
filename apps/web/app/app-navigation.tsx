"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useLayoutEffect, useRef } from "react";

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
        return (
          <Link
            className={current ? "is-current" : undefined}
            href={destination.href}
            aria-current={current ? "page" : undefined}
            key={destination.href}
            onClick={retainScrollPosition}
          >
            {destination.label}
          </Link>
        );
      })}
    </nav>
  );
}
