import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { StrategyInitializationAssessment } from "./strategy-initialization-readiness";
import {
  AutomationNextActionPanel,
  selectAutomationNextAction,
  type AutomationNextActionInput,
} from "./automation-next-action";

const readyAssessment: StrategyInitializationAssessment = {
  status: "READY_TO_INITIALIZE",
  eligible: true,
  checks: [],
};

const base: AutomationNextActionInput = {
  automationId: "mandate-1",
  automationType: "AI_AUTONOMOUS",
  autonomyLevel: "FULL_AUTONOMOUS",
  executionMode: "SHADOW",
  assessment: readyAssessment,
  scheduleConfigured: false,
  schedulerEnabled: false,
};

function blocked(
  code: string,
  detail = "This exact check must pass first.",
): StrategyInitializationAssessment {
  return {
    status: "BLOCKED",
    eligible: false,
    checks: [
      {
        code,
        label: code,
        state: "BLOCKED",
        blocking: true,
        detail,
      },
    ],
  };
}

describe("AutomationNextActionPanel", () => {
  afterEach(cleanup);

  it("sends a draft to its exact mandate lifecycle control", () => {
    const action = selectAutomationNextAction({
      ...base,
      assessment: blocked("MANDATE_VERSION"),
    });
    render(<AutomationNextActionPanel action={action} />);

    expect(
      screen.getByRole("heading", {
        name: "Review and mark this version Ready",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /Go to lifecycle controls/i }),
    ).toHaveAttribute("href", "#mandate-lifecycle-controls");
    expect(
      screen.getByText(/does not initialize an engine/i),
    ).toBeInTheDocument();
  });

  it("routes a conflicting capital claim to the Capital Center", () => {
    const action = selectAutomationNextAction({
      ...base,
      assessment: blocked(
        "ACCOUNT_CAPACITY",
        "The existing exclusive claim blocks this budget.",
      ),
    });
    render(<AutomationNextActionPanel action={action} />);

    expect(
      screen.getByRole("heading", { name: "Resolve the capital claim" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /Capital Center/i }),
    ).toHaveAttribute("href", "/capital");
  });

  it("offers initialization only when the exact readiness assessment passes", () => {
    const action = selectAutomationNextAction(base);
    render(<AutomationNextActionPanel action={action} />);

    expect(
      screen.getByRole("heading", { name: "Initialize the AI Shadow Engine" }),
    ).toBeInTheDocument();
    expect(screen.getByText(/never a broker order/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /initialize/i })).toHaveAttribute(
      "href",
      "#mandate-lifecycle-controls",
    );
  });

  it("shows the next server-side cycle for an active healthy schedule", () => {
    const action = selectAutomationNextAction({
      ...base,
      instance: {
        id: "instance-1",
        status: "ACTIVE",
        current_state: "AI_MONITORING",
      },
      schedule: {
        last_status: "SUCCEEDED",
        consecutive_failures: 0,
        next_run_at: "2026-09-01T01:00:00Z",
      },
      scheduleConfigured: true,
      schedulerEnabled: true,
    });
    render(<AutomationNextActionPanel action={action} />);

    expect(
      screen.getByRole("heading", { name: "Let the next guarded cycle run" }),
    ).toBeInTheDocument();
    expect(screen.getByText(/Sep 1.*UTC/i)).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /decision journal/i }),
    ).toHaveAttribute("href", "#decision-journal");
  });

  it("prioritizes failed schedule evidence over the next run", () => {
    const action = selectAutomationNextAction({
      ...base,
      instance: { id: "instance-1", status: "ACTIVE" },
      schedule: {
        last_status: "FAILED",
        consecutive_failures: 2,
        next_run_at: "2026-09-01T01:00:00Z",
      },
      scheduleConfigured: true,
      schedulerEnabled: true,
    });
    render(<AutomationNextActionPanel action={action} />);

    expect(
      screen.getByRole("heading", {
        name: "Inspect the failed non-live cycle",
      }),
    ).toBeInTheDocument();
    expect(screen.getByText(/2 consecutive failures/i)).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /schedule health/i }),
    ).toHaveAttribute("href", "#schedule-controls");
  });

  it("keeps a paused instance owner-controlled", () => {
    const action = selectAutomationNextAction({
      ...base,
      instance: { id: "instance-1", status: "PAUSED" },
    });
    render(<AutomationNextActionPanel action={action} />);

    expect(
      screen.getByRole("heading", { name: "Resume only when you are ready" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /resume controls/i }),
    ).toHaveAttribute("href", "#strategy-instance-controls");
  });

  it("requires a new immutable version after a completed instance", () => {
    const action = selectAutomationNextAction({
      ...base,
      instance: { id: "instance-1", status: "COMPLETED" },
    });
    render(<AutomationNextActionPanel action={action} />);

    expect(
      screen.getByRole("heading", {
        name: "Create and review the next version",
      }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: /version controls/i }),
    ).toHaveAttribute("href", "#configuration-controls");
  });

  it("labels the panel as guidance with no order authority", () => {
    render(
      <AutomationNextActionPanel action={selectAutomationNextAction(base)} />,
    );

    expect(
      screen.getByText(
        /Guidance only · no provider call · no order authority/i,
      ),
    ).toBeInTheDocument();
  });
});
