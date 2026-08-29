import { describe, expect, it } from "vitest";

import {
  projectPinnedAIRuntime,
  scheduleMatchesPinnedAIRuntime,
} from "./runtime-contract";

const mandate = {
  id: "mandate-1",
  current_version: 7,
  status: "DRAFT",
};

const instance = {
  id: "instance-1",
  automation_mandate_id: "mandate-1",
  mandate_version: 6,
  financial_account_id: "account-1",
  capital_bucket_id: "bucket-1",
  strategy_identifier: "ai_shadow",
  execution_mode: "SHADOW",
  status: "ACTIVE",
};

const snapshot = {
  financial_account_id: "account-1",
  capital_bucket_id: "bucket-1",
  automation_type: "AI_AUTONOMOUS",
  status: "READY",
  execution_mode: "SHADOW",
  autonomy_level: "FULL_AUTONOMOUS",
  ai_provider_connection_id: "ai-connection-1",
  ai_model_id: "gpt-5.6-sol",
  strategy_parameters: {
    objective: "Preserve capital while observing BTC.",
    max_proposal_notional: "1.0000000000",
  },
  risk_parameters: { max_trades_per_day: 1 },
  allowed_universe: { symbols: ["BTC", "ETH"], universe_ids: [] },
  prohibited_universe: { symbols: [] },
  margin_allowed: false,
  options_allowed: false,
  schedule_conditions: {
    enabled: true,
    interval_minutes: 60,
    session: "CONTINUOUS",
  },
  execution_capable: false,
};

const version = {
  ID: "version-6",
  MandateID: "mandate-1",
  VersionNumber: 6,
  Snapshot: snapshot,
};

describe("projectPinnedAIRuntime", () => {
  it("projects only the exact immutable version pinned to the instance", () => {
    const contract = projectPinnedAIRuntime({
      mandate,
      instance,
      versionAvailable: true,
      version,
    });

    expect(contract.bindingValid).toBe(true);
    expect(contract.contextAvailable).toBe(true);
    expect(contract.pinnedVersion).toBe(6);
    expect(contract.currentVersion).toBe(7);
    expect(contract.newerDraftAvailable).toBe(true);
    expect(contract.configuration).toBe(snapshot);
    expect(contract.symbols).toEqual(["BTC", "ETH"]);
    expect(contract.maxProposalNotional).toBe("1.0000000000");
    expect(contract.maxTradesPerDay).toBe(1);
    expect(contract.legacyDailyActionLimitMissing).toBe(false);
    expect(contract.scheduleSession).toBe("CONTINUOUS");
  });

  it("accepts an exact pinned AI Paper runtime without weakening non-live mode matching", () => {
    const paperContract = projectPinnedAIRuntime({
      mandate,
      instance: { ...instance, execution_mode: "PAPER" },
      versionAvailable: true,
      version: {
        ...version,
        Snapshot: { ...snapshot, execution_mode: "PAPER" },
      },
    });

    expect(paperContract.bindingValid).toBe(true);
    expect(paperContract.configuration?.execution_mode).toBe("PAPER");
    expect(
      projectPinnedAIRuntime({
        mandate,
        instance,
        versionAvailable: true,
        version: {
          ...version,
          Snapshot: { ...snapshot, execution_mode: "PAPER" },
        },
      }).bindingValid,
    ).toBe(false);
  });

  it("fails closed when the version read is unavailable", () => {
    const contract = projectPinnedAIRuntime({
      mandate,
      instance,
      versionAvailable: false,
    });

    expect(contract.contextAvailable).toBe(false);
    expect(contract.bindingValid).toBe(false);
    expect(contract.configuration).toBeUndefined();
    expect(contract.symbols).toEqual([]);
    expect(contract.maxProposalNotional).toBeUndefined();
    expect(contract.legacyDailyActionLimitMissing).toBe(false);
  });

  it("preserves an explicit legacy absence without inventing a daily ceiling", () => {
    const contract = projectPinnedAIRuntime({
      mandate,
      instance,
      versionAvailable: true,
      version: {
        ...version,
        Snapshot: { ...snapshot, risk_parameters: {} },
      },
    });

    expect(contract.bindingValid).toBe(true);
    expect(contract.maxTradesPerDay).toBeUndefined();
    expect(contract.legacyDailyActionLimitMissing).toBe(true);
  });

  it("rejects a wrapper or snapshot that does not match the pinned instance", () => {
    for (const invalidVersion of [
      { ...version, VersionNumber: 5 },
      {
        ...version,
        Snapshot: { ...snapshot, financial_account_id: "other-account" },
      },
      {
        ...version,
        Snapshot: { ...snapshot, capital_bucket_id: "other-bucket" },
      },
      {
        ...version,
        Snapshot: { ...snapshot, execution_mode: "LIVE" },
      },
      {
        ...version,
        Snapshot: { ...snapshot, execution_capable: true },
      },
    ]) {
      expect(
        projectPinnedAIRuntime({
          mandate,
          instance,
          versionAvailable: true,
          version: invalidVersion,
        }).bindingValid,
      ).toBe(false);
    }

    expect(
      projectPinnedAIRuntime({
        mandate,
        instance: { ...instance, strategy_identifier: "wheel" },
        versionAvailable: true,
        version,
      }).bindingValid,
    ).toBe(false);
  });

  it("rejects inferred, malformed, or out-of-bounds AI controls", () => {
    for (const invalidSnapshot of [
      {
        ...snapshot,
        allowed_universe: { symbols: ["btc"] },
      },
      {
        ...snapshot,
        strategy_parameters: {
          objective: "Preserve capital.",
          max_proposal_notional: "0.0000000000",
        },
      },
      {
        ...snapshot,
        strategy_parameters: {
          objective: "Preserve capital.",
          max_proposal_notional: "1.00000000001",
        },
      },
      {
        ...snapshot,
        risk_parameters: { max_trades_per_day: 49 },
      },
      {
        ...snapshot,
        risk_parameters: { max_trades_per_day: "1" },
      },
      {
        ...snapshot,
        risk_parameters: undefined,
      },
      {
        ...snapshot,
        risk_parameters: {
          max_trades_per_day: 1,
          max_daily_loss: "10.0000000000",
        },
      },
      {
        ...snapshot,
        schedule_conditions: {
          enabled: true,
          interval_minutes: 15,
          session: "CONTINUOUS",
        },
      },
    ]) {
      expect(
        projectPinnedAIRuntime({
          mandate,
          instance,
          versionAvailable: true,
          version: { ...version, Snapshot: invalidSnapshot },
        }).bindingValid,
      ).toBe(false);
    }
  });
});

describe("scheduleMatchesPinnedAIRuntime", () => {
  const contract = projectPinnedAIRuntime({
    mandate,
    instance,
    versionAvailable: true,
    version,
  });
  const envelope = {
    live_execution_available: false,
    schedule: {
      enabled: true,
      strategy_instance_id: "instance-1",
      mandate_id: "mandate-1",
      mandate_version: 6,
      interval_minutes: 60,
      session: "CONTINUOUS",
    },
  };

  it("matches only the schedule row pinned to the same immutable version", () => {
    expect(
      scheduleMatchesPinnedAIRuntime({
        contract,
        instanceID: "instance-1",
        mandateID: "mandate-1",
        scheduleAvailable: true,
        envelope,
      }),
    ).toBe(true);

    expect(
      scheduleMatchesPinnedAIRuntime({
        contract,
        instanceID: "instance-1",
        mandateID: "mandate-1",
        scheduleAvailable: true,
        envelope: {
          ...envelope,
          schedule: { ...envelope.schedule, mandate_version: 7 },
        },
      }),
    ).toBe(false);
    expect(
      scheduleMatchesPinnedAIRuntime({
        contract,
        instanceID: "instance-1",
        mandateID: "mandate-1",
        scheduleAvailable: true,
        envelope: {
          ...envelope,
          schedule: { ...envelope.schedule, strategy_instance_id: undefined },
        },
      }),
    ).toBe(false);
  });

  it("fails closed on a partial envelope or unexpected live capability", () => {
    expect(
      scheduleMatchesPinnedAIRuntime({
        contract,
        instanceID: "instance-1",
        mandateID: "mandate-1",
        scheduleAvailable: false,
      }),
    ).toBe(false);
    expect(
      scheduleMatchesPinnedAIRuntime({
        contract,
        instanceID: "instance-1",
        mandateID: "mandate-1",
        scheduleAvailable: true,
        envelope: { ...envelope, live_execution_available: true },
      }),
    ).toBe(false);
  });

  it("accepts an owner-invoked contract only for the exact instance", () => {
    const ownerInvokedContract = projectPinnedAIRuntime({
      mandate,
      instance,
      versionAvailable: true,
      version: {
        ...version,
        Snapshot: {
          ...snapshot,
          schedule_conditions: { enabled: false },
        },
      },
    });

    for (const [strategyInstanceID, expected] of [
      ["instance-1", true],
      ["other-instance", false],
    ] as const) {
      expect(
        scheduleMatchesPinnedAIRuntime({
          contract: ownerInvokedContract,
          instanceID: "instance-1",
          mandateID: "mandate-1",
          scheduleAvailable: true,
          envelope: {
            live_execution_available: false,
            schedule: {
              enabled: false,
              strategy_instance_id: strategyInstanceID,
            },
          },
        }),
      ).toBe(expected);
    }
  });
});
