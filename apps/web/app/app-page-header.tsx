import Link from "next/link";
import type { ReactNode } from "react";

import { ArbionBrand } from "./brand";
import { AppNavigation } from "./app-navigation";

type AppPageHeaderProps = {
  backHref?: string;
  backLabel?: string;
  actions?: ReactNode;
  contentHeadingId?: string;
};

export function AppPageHeader({
  backHref = "/dashboard",
  backLabel = "Dashboard",
  actions,
  contentHeadingId,
}: AppPageHeaderProps) {
  return (
    <>
      <header className="app-page-header">
        <a className="app-skip-link" href="#app-main-content">
          Skip to main content
        </a>
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
      <span
        aria-label={contentHeadingId ? undefined : "Main content start"}
        aria-labelledby={contentHeadingId}
        className="app-main-content-target"
        id="app-main-content"
        tabIndex={-1}
      />
    </>
  );
}
