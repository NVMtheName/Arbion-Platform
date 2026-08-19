import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const navigation = vi.hoisted(() => ({ refresh: vi.fn() }));
vi.mock("next/navigation", () => ({
  useRouter: () => ({ refresh: navigation.refresh }),
}));

import { PaperOptionsSimulationAttestationControls } from "./paper-options-simulation-attestation-controls";

const base = {
  automationId: "mandate-1",
  currentVersion: 6,
  automationType: "STRATEGY",
  executionMode: "PAPER",
  optionsAllowed: true,
  capabilityUnverified: true,
  attested: false,
  hasActiveInstance: false,
};

describe("PaperOptionsSimulationAttestationControls", () => {
  afterEach(() => {
    cleanup();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("creates only a separately confirmed PAPER draft", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true });
    vi.stubGlobal("fetch", fetchMock);
    render(<PaperOptionsSimulationAttestationControls {...base} />);

    expect(
      screen.getByText(/schwab's options capability is UNKNOWN/i),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(
      screen.getByRole("button", { name: /create attested paper draft/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/automations/mandate-1/paper-options-simulation-attestation",
      {
        method: "PATCH",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ expected_version: 6, attested: true }),
      },
    );
    expect(await screen.findByRole("status")).toHaveTextContent(
      /capability remains UNKNOWN/i,
    );
  });

  it("cannot change while a non-live capital claim exists", () => {
    render(
      <PaperOptionsSimulationAttestationControls {...base} hasActiveInstance />,
    );
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(
      screen.getByText(/finish the active or paused/i),
    ).toBeInTheDocument();
  });

  it("is absent outside an unknown-capability PAPER strategy", () => {
    const { rerender } = render(
      <PaperOptionsSimulationAttestationControls
        {...base}
        executionMode="SHADOW"
      />,
    );
    expect(screen.queryByRole("region")).not.toBeInTheDocument();
    rerender(
      <PaperOptionsSimulationAttestationControls
        {...base}
        executionMode="PAPER"
        capabilityUnverified={false}
      />,
    );
    expect(screen.queryByRole("region")).not.toBeInTheDocument();
  });

  it("supports an explicit audited revocation", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true });
    vi.stubGlobal("fetch", fetchMock);
    render(<PaperOptionsSimulationAttestationControls {...base} attested />);
    fireEvent.click(screen.getByRole("checkbox"));
    fireEvent.click(
      screen.getByRole("button", { name: /create draft without attestation/i }),
    );
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({
      body: JSON.stringify({ expected_version: 6, attested: false }),
    });
  });
});
