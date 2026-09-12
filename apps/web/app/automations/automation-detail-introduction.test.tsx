import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  AutomationDetailIntroduction,
  automationDetailTitle,
} from "./automation-detail-introduction";
import { MandateIdentitySummary } from "./mandate-identity-summary";
import { AutomationNextActionPanel } from "./automation-next-action";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("Automation detail overview", () => {
  it.each([
    ["AI_AUTONOMOUS", "PAPER", "AI Paper Engine"],
    ["AI_AUTONOMOUS", "SHADOW", "AI Shadow Engine"],
    ["AI_AUTONOMOUS", "LIVE", "AI Engine"],
    ["AI_AUTONOMOUS", "BACKTEST", "AI Engine"],
    ["AI_AUTONOMOUS", "UNRECOGNIZED", "AI Engine"],
    ["AI_AUTONOMOUS", "", "AI Engine"],
    ["WHEEL", "PAPER", "WHEEL"],
  ])(
    "labels %s / %s without inventing a supported mode",
    (type, mode, title) => {
      expect(automationDetailTitle(type, mode)).toBe(title);
      render(
        <AutomationDetailIntroduction
          automationType={type}
          executionMode={mode}
        />,
      );
      expect(
        screen.getByRole("heading", { level: 1, name: title }),
      ).toHaveAttribute("id", "automation-page-title");
    },
  );
  it("omits unavailable-page anchors and retains an accessible heading", () => {
    render(<AutomationDetailIntroduction available={false} />);
    expect(
      screen.getByRole("heading", { name: "Mandate unavailable" }),
    ).toHaveAttribute("id", "automation-page-title");
    expect(screen.queryByRole("navigation")).not.toBeInTheDocument();
  });
  it("links only to overview evidence and preserves the existing next action without requests", () => {
    const request = vi.fn();
    vi.stubGlobal("fetch", request);
    render(
      <>
        <AutomationDetailIntroduction
          automationType="AI_AUTONOMOUS"
          executionMode="PAPER"
        />
        <MandateIdentitySummary
          id="mandate-identity"
          mandateId="test-mandate"
          automationType="AI_AUTONOMOUS"
          financialAccountId="test-account"
          capitalBucketId="test-bucket"
          strategyIdentifier=""
          aiModelId="gpt-5.6-sol"
          autonomyLevel="FULL_AUTONOMOUS"
          executionMode="PAPER"
          status="READY"
          currentVersion={3}
        />
        <AutomationNextActionPanel
          id="automation-next-step"
          action={{
            tone: "ATTENTION",
            eyebrow: "SCHEDULE NEEDS REVIEW",
            title: "Inspect the failed non-live cycle",
            detail: "Saved failure evidence",
            href: "#existing-schedule",
            actionLabel: "Review schedule health",
          }}
        />
        <section id="existing-schedule">Existing unchanged controls</section>
      </>,
    );
    const links = within(
      screen.getByRole("navigation", { name: "Automation sections" }),
    ).getAllByRole("link");
    expect(links.map((link) => link.getAttribute("href"))).toEqual([
      "#automation-overview",
      "#mandate-identity",
      "#automation-next-step",
    ]);
    for (const link of links)
      expect(document.querySelector(link.getAttribute("href")!)).not.toBeNull();
    expect(
      screen.getByRole("link", { name: /Review schedule health/ }),
    ).toHaveAttribute("href", "#existing-schedule");
    expect(
      screen.getByRole("region", { name: "Inspect the failed non-live cycle" }),
    ).toHaveClass("is-attention");
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(request).not.toHaveBeenCalled();
  });
});
