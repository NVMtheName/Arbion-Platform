import Link from "next/link";
import type { ReactNode } from "react";

import { ArbionBrand } from "./brand";
import { AppNavigation } from "./app-navigation";

type AppPageHeaderProps = {
  backHref?: string;
  backLabel?: string;
  actions?: ReactNode;
};

export function AppPageHeader({
  backHref = "/dashboard",
  backLabel = "Dashboard",
  actions,
}: AppPageHeaderProps) {
  return (
    <header className="app-page-header">
      <ArbionBrand className="section-brand" href="/dashboard" priority />
      <AppNavigation />
      <div className="app-page-header-actions">
        {actions ?? (
          <Link className="app-back-link" href={backHref}>
            ← {backLabel}
          </Link>
        )}
      </div>
    </header>
  );
}
