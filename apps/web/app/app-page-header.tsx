import Link from "next/link";
import type { ReactNode } from "react";

import { ArbionBrand } from "./brand";
import { AppNavigation } from "./app-navigation";

type AppPageHeaderProps = {
  backHref?: string;
  backLabel?: string;
  actions?: ReactNode;
  contentHeadingId?: string;
  showConnectionHealth?: boolean;
};

export function AppPageHeader({
  backHref,
  backLabel,
  actions,
  contentHeadingId,
  showConnectionHealth = true,
}: AppPageHeaderProps) {
  return (
    <>
      <header className="app-page-header">
        <a className="app-skip-link" href="#app-main-content">
          Skip to main content
        </a>
        <ArbionBrand className="section-brand" href="/dashboard" priority />
        <AppNavigation showConnectionHealth={showConnectionHealth} />
        <div className="app-page-header-actions">
          {actions ??
            (backHref && backHref !== "/dashboard" && backLabel ? (
              <Link className="app-back-link" href={backHref}>
                ← {backLabel}
              </Link>
            ) : null)}
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
