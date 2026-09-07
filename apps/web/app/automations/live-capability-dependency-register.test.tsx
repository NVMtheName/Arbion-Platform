import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
  LiveCapabilityDependencyRegister,
  projectLiveCapabilityDependencyRegister,
} from "./live-capability-dependency-register";
import type {
  PaperLiveReadinessEngineProjection,
  PaperLiveReadinessSignal,
} from "./paper-live-readiness-dossier";

function signal(
  key: string,
  state: PaperLiveReadinessSignal["state"] = "PROVEN",
): PaperLiveReadinessSignal {
  return {
    key,
    label: `${key} label`,
    state,
    detail: `${key} exact detail`,
    evidence: `${key} exact saved evidence`,
  };
}

function engine(): PaperLiveReadinessEngineProjection {
  return {
    id: "11111111-1111-4111-8111-111111111111",
    instanceID: "22222222-2222-4222-8222-222222222222",
    title: "Coinbase AI Paper",
    accountName: "Coinbase Portfolio",
    provider: "coinbase",
    observedAt: "2026-09-07T14:00:00Z",
    financialAccountID: "33333333-3333-4333-8333-333333333333",
    financialConnectionID: "44444444-4444-4444-8444-444444444444",
    capitalBucketID: "55555555-5555-4555-8555-555555555555",
    capitalReservationID: "66666666-6666-4666-8666-666666666666",
    latestDecisionID: "77777777-7777-4777-8777-777777777777",
    latestDecisionAt: "2026-09-07T13:26:52Z",
    modelRoute: "openai / gpt-5.6-sol / deep",
    status: "PLATFORM_BLOCKED",
    ownerAction: "Live remains unavailable.",
    signals: [
      signal("AUTHORIZATION"),
      signal("CAPITAL"),
      signal("SCHEDULER"),
      signal("PROVENANCE"),
      signal("BOUNDARY"),
      signal("PAPER_EVIDENCE"),
      signal("ACCOUNTING"),
      signal("SHADOW_EVIDENCE"),
      signal("LIVE_CAPABILITY", "BLOCKED"),
    ],
    shadowInstanceID: "88888888-8888-4888-8888-888888888888",
    shadowMandateID: "99999999-9999-4999-8999-999999999999",
    detailHref:
      "/automations/11111111-1111-4111-8111-111111111111#runtime-evidence",
  };
}

describe("Live capability dependency register", () => {
  it("keeps live blocked after proving the complete non-live evidence case", () => {
    const register = projectLiveCapabilityDependencyRegister(engine());

    expect(register).toEqual(
      expect.objectContaining({
        status: "LIVE_BLOCKED",
        dependencyCount: 9,
        provenNonLiveCount: 1,
        structuralBlockerCount: 8,
        separateReviewCount: 2,
        unavailableCount: 1,
        attention: false,
      }),
    );
    expect(register.packetFingerprint).toMatch(/^[0-9a-f]{64}$/);
    expect(JSON.parse(register.packetJSON)).toEqual(
      expect.objectContaining({
        schema_version: "arbion.live_capability_dependency_packet.v1",
        financial_account_id: "33333333-3333-4333-8333-333333333333",
        live_execution_available: false,
        live_promotion_available: false,
        grants_authority: false,
        content_sha256: register.packetFingerprint,
      }),
    );
  });

  it("produces a deterministic fingerprint that changes with exact evidence", () => {
    const current = engine();
    const same = engine();
    const collecting = engine();
    collecting.signals = collecting.signals.map((item) =>
      item.key === "PAPER_EVIDENCE" ? { ...item, state: "COLLECTING" } : item,
    );

    expect(
      projectLiveCapabilityDependencyRegister(current).packetFingerprint,
    ).toBe(projectLiveCapabilityDependencyRegister(same).packetFingerprint);
    expect(
      projectLiveCapabilityDependencyRegister(collecting).packetFingerprint,
    ).not.toBe(
      projectLiveCapabilityDependencyRegister(current).packetFingerprint,
    );
    expect(
      projectLiveCapabilityDependencyRegister(collecting).collectingCount,
    ).toBe(1);
  });

  it("opens exact dependency evidence when the non-live case needs review", () => {
    const item = engine();
    item.signals = item.signals.map((entry) =>
      entry.key === "SCHEDULER" ? { ...entry, state: "BLOCKED" } : entry,
    );

    const register = projectLiveCapabilityDependencyRegister(item);

    expect(register.attention).toBe(true);
    expect(register.dependencies[0]).toEqual(
      expect.objectContaining({
        key: "NON_LIVE_EVIDENCE_CASE",
        state: "BLOCKED",
        structuralBlocker: false,
      }),
    );
  });

  it("renders a downloadable non-executable packet without a live action", () => {
    render(<LiveCapabilityDependencyRegister engine={engine()} />);

    const register = screen.getByRole("region", {
      name: "Coinbase AI Paper live capability dependency register",
    });
    expect(register).toHaveTextContent("No live promotion path exists.");
    expect(
      screen.getByText("Structural blockers").nextElementSibling,
    ).toHaveTextContent("8");
    expect(register).toHaveTextContent("Not implemented");
    expect(register).toHaveTextContent("Review not recorded");
    expect(register).toHaveTextContent("grants_authority");
    const download = screen.getByRole("link", {
      name: "Download exact JSON packet",
    });
    expect(download).toHaveAttribute(
      "download",
      "arbion-live-capability-22222222-2222-4222-8222-222222222222.json",
    );
    expect(download.getAttribute("href")).toMatch(
      /^data:application\/json;charset=utf-8,/,
    );
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});
