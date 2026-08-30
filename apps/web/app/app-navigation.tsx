"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

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

function isCurrent(pathname: string, matches: readonly string[]) {
  return matches.some(
    (match) => pathname === match || pathname.startsWith(`${match}/`),
  );
}

export function AppNavigation({ className = "" }: { className?: string }) {
  const pathname = usePathname();
  return (
    <nav
      className={["app-navigation", className].filter(Boolean).join(" ")}
      aria-label="Application navigation"
    >
      {destinations.map((destination) => {
        const current = isCurrent(pathname, destination.matches);
        return (
          <Link
            className={current ? "is-current" : undefined}
            href={destination.href}
            aria-current={current ? "page" : undefined}
            key={destination.href}
          >
            {destination.label}
          </Link>
        );
      })}
    </nav>
  );
}
