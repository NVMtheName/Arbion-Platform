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

import { AIShadowParameterControls } from "./ai-shadow-parameter-controls";

describe("AIShadowParameterControls", () => {
  afterEach(() => {
    cleanup();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("creates a new immutable draft without enabling execution", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(
      <AIShadowParameterControls
        automationId="mandate-1"
        currentVersion={4}
        status="READY"
        hasActiveInstance
        parameters={{
          objective: "Preserve capital.",
          max_proposal_notional: "1",
        }}
      />,
    );

    fireEvent.change(screen.getByLabelText(/maximum hypothetical proposal/i), {
      target: { value: "1000" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: /save ai shadow controls/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/automations/mandate-1/ai-shadow-parameters",
      {
        method: "PATCH",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          expected_version: 4,
          strategy_parameters: {
            objective: "Preserve capital.",
            max_proposal_notional: "1000",
          },
        }),
      },
    );
    expect(await screen.findByText(/new immutable draft/i)).toBeInTheDocument();
    expect(screen.getByText(/never grants broker-write/i)).toBeInTheDocument();
    expect(navigation.refresh).toHaveBeenCalledTimes(1);
  });

  it("keeps archived mandates immutable", () => {
    render(
      <AIShadowParameterControls
        automationId="mandate-1"
        currentVersion={4}
        status="ARCHIVED"
        hasActiveInstance={false}
        parameters={{
          objective: "Preserve capital.",
          max_proposal_notional: "1",
        }}
      />,
    );

    expect(
      screen.getByRole("button", { name: /save ai shadow controls/i }),
    ).toBeDisabled();
  });
});
