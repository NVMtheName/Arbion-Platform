"use client";

import Link from "next/link";
import { useState } from "react";

type CopyStatus = "IDLE" | "COPIED" | "FAILED";

export function DecisionPermalink({ href }: { href: string }) {
  const [status, setStatus] = useState<CopyStatus>("IDLE");

  async function copyExactLink() {
    try {
      if (!navigator.clipboard?.writeText) throw new Error("unavailable");
      await navigator.clipboard.writeText(
        new URL(href, window.location.origin).toString(),
      );
      setStatus("COPIED");
    } catch {
      setStatus("FAILED");
    }
  }

  return (
    <div className="journal-entry-permalink">
      <Link href={href}>Open exact record</Link>
      <button type="button" onClick={copyExactLink}>
        {status === "COPIED" ? "Link copied" : "Copy exact link"}
      </button>
      <span
        aria-live="polite"
        className={status === "FAILED" ? "is-error" : undefined}
      >
        {status === "COPIED"
          ? "The durable owner-scoped record link is ready."
          : status === "FAILED"
            ? "Copy is unavailable. Open the exact record and copy its address instead."
            : ""}
      </span>
    </div>
  );
}
