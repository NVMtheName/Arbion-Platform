import { readFileSync } from "node:fs";

import { describe, expect, it } from "vitest";

const appSource = (relativePath: string) =>
  readFileSync(new URL(relativePath, import.meta.url), "utf8");

describe("signed-in command content continuity", () => {
  it.each([
    ["Portfolio", "./accounts/page.tsx"],
    ["Markets", "./markets/page.tsx"],
    ["Automations", "./automations/page.tsx"],
    ["Connections", "./settings/connections/page.tsx"],
  ])("keeps %s inside the shared viewport contract", (_label, source) => {
    expect(appSource(source)).toContain("command-content-continuity");
  });

  it("contains the page while preserving independently scrollable evidence", () => {
    const styles = appSource("./styles.css");

    expect(styles).toMatch(
      /\.command-content-continuity\s*\{[^}]*overflow-x:\s*clip;/,
    );
    expect(styles).toMatch(
      /\.command-content-continuity[^}]*min-inline-size:\s*0;/,
    );
    expect(styles).toMatch(
      /\.command-content-continuity\s+:where\(dd, code\)[^}]*overflow-wrap:\s*anywhere;/,
    );
    expect(styles).toMatch(
      /\.command-data-scroll\s*\{[^}]*overflow-x:\s*auto;[^}]*overscroll-behavior-inline:\s*contain;/,
    );
    expect(styles).toMatch(
      /@media \(max-width:\s*760px\)[\s\S]*?\.command-data-scroll-hint\s*\{[^}]*position:\s*static;/,
    );
  });

  it("keeps signed-in controls touch-friendly and keyboard-visible", () => {
    const styles = appSource("./styles.css");

    expect(styles).toMatch(
      /\.command-content-continuity[\s\S]*?>\s*:not\(\.app-page-header\)[\s\S]*?:where\([\s\S]*?button,[\s\S]*?summary,[\s\S]*?a\.button-link,[\s\S]*?\.connection-steps a[\s\S]*?\)\s*\{[^}]*touch-action:\s*manipulation;/,
    );
    expect(styles).toMatch(
      /\.command-content-continuity[\s\S]*?>\s*:not\(\.app-page-header\)[\s\S]*?:where\([\s\S]*?\):focus-visible\s*\{[^}]*outline:\s*2px solid var\(--brand-cyan\);[^}]*outline-offset:\s*3px;/,
    );
    expect(styles).toMatch(
      /@media \(max-width:\s*760px\)[\s\S]*?\.command-content-continuity[\s\S]*?:where\([\s\S]*?button,[\s\S]*?summary,[\s\S]*?\.connection-steps a[\s\S]*?\)\s*\{[^}]*min-block-size:\s*44px;/,
    );
    expect(styles).toMatch(
      /\.command-data-scroll\s*\{[^}]*scrollbar-color:[^;]+;[^}]*scrollbar-width:\s*thin;/,
    );
    expect(styles).toMatch(
      /@media \(max-width:\s*760px\)[\s\S]*?\.command-data-scroll::-webkit-scrollbar-thumb\s*\{[^}]*background:\s*rgba\(124, 229, 236, 0\.72\);/,
    );
  });
});
