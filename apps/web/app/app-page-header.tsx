import Link from "next/link";

import { ArbionBrand } from "./brand";

type AppPageHeaderProps = {
  backHref?: string;
  backLabel?: string;
};

export function AppPageHeader({
  backHref = "/dashboard",
  backLabel = "Dashboard",
}: AppPageHeaderProps) {
  return (
    <header className="app-page-header">
      <ArbionBrand className="section-brand" href="/dashboard" priority />
      <Link className="app-back-link" href={backHref}>
        ← {backLabel}
      </Link>
    </header>
  );
}
