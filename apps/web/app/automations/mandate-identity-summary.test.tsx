import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { MandateIdentitySummary } from "./mandate-identity-summary";

describe("mandate identity summary", () => {
  afterEach(cleanup);

  it("leads with human-readable AI account and safeguard details", () => {
    render(
      <MandateIdentitySummary
        mandateId="mandate-123"
        automationType="AI_AUTONOMOUS"
        financialAccountId="account-123"
        financialAccount={{
          display_name: "Primary Coinbase",
          provider: "coinbase",
        }}
        capitalBucketId="bucket-123"
        capitalBucket={{
          name: "AI Shadow budget",
          allocation_type: "FIXED_AMOUNT",
          allocation_value: "25",
          currency: "USD",
        }}
        strategyIdentifier=""
        aiModelId="gpt-5.6-sol"
        autonomyLevel="FULL_AUTONOMOUS"
        executionMode="SHADOW"
        status="READY"
        currentVersion={2}
        strategyInstanceId="instance-123"
      />,
    );

    expect(
      screen.getByRole("link", { name: "Primary Coinbase" }),
    ).toHaveAttribute("href", "/accounts/account-123");
    expect(screen.getByText("Coinbase")).toBeInTheDocument();
    expect(screen.getByText("AI Shadow Engine")).toBeInTheDocument();
    expect(screen.getByText("AI Shadow budget")).toBeInTheDocument();
    expect(screen.getByText("USD 25 fixed allocation")).toBeInTheDocument();
    expect(screen.getByText("gpt-5.6-sol")).toBeInTheDocument();
    expect(screen.getByText("Full Autonomous")).toBeInTheDocument();
    expect(screen.getByText("Shadow only")).toBeInTheDocument();
    expect(screen.getByText("No broker order can be sent")).toBeInTheDocument();
    expect(screen.getByText("Mandate version 2")).toBeInTheDocument();
  });

  it("keeps exact identifiers in a collapsed technical reference section", () => {
    render(
      <MandateIdentitySummary
        mandateId="mandate-123"
        automationType="AI_AUTONOMOUS"
        financialAccountId="account-123"
        capitalBucketId="bucket-123"
        strategyIdentifier=""
        aiModelId="gpt-5.6-sol"
        autonomyLevel="FULL_AUTONOMOUS"
        executionMode="SHADOW"
        status="READY"
        currentVersion={2}
        strategyInstanceId="instance-123"
      />,
    );

    const details = screen.getByText("Technical references").closest("details");
    expect(details).not.toHaveAttribute("open");
    expect(screen.getByText("mandate-123")).toBeInTheDocument();
    expect(screen.getByText("account-123")).toBeInTheDocument();
    expect(screen.getByText("bucket-123")).toBeInTheDocument();
    expect(screen.getByText("instance-123")).toBeInTheDocument();
  });
});
