import type { ReactNode } from "react";

import { ArbionBrand } from "./brand";

export function AuthEntryHeader({
  step,
  title,
  description,
}: {
  step: string;
  title: ReactNode;
  description?: ReactNode;
}) {
  return (
    <header className="auth-entry-header">
      <ArbionBrand className="auth-brand" href="/" priority />
      <p className="eyebrow">{step}</p>
      <h1 id="auth-entry-title">{title}</h1>
      {description && <p className="lede">{description}</p>}
    </header>
  );
}
