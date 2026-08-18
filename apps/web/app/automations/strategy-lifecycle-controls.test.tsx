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

import { StrategyLifecycleControls } from "./strategy-lifecycle-controls";

const base = {
  instanceId: "instance-1",
  currentState: "SHORT_PUT_OPEN",
  stateVersion: 2,
  status: "ACTIVE",
  executionMode: "PAPER",
  strategyIdentifier: "wheel",
};

describe("StrategyLifecycleControls", () => {
  afterEach(() => {
    cleanup();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("requires an explicit PAPER acknowledgement and records an owner event", async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: true });
    vi.stubGlobal("fetch", fetchMock);
    render(<StrategyLifecycleControls {...base} />);

    const submit = screen.getByRole("button", {
      name: /record paper event/i,
    });
    expect(submit).toBeDisabled();
    fireEvent.change(screen.getByLabelText(/outcome/i), {
      target: { value: "ASSIGNED" },
    });
    fireEvent.click(screen.getByLabelText(/confirm this is a paper/i));
    fireEvent.click(submit);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/strategy-instances/instance-1/lifecycle-events",
    );
    const body = JSON.parse(String(fetchMock.mock.calls[0][1]?.body));
    expect(body).toMatchObject({
      event_type: "ASSIGNED",
      expected_state_version: 2,
      confirm_paper_simulation: true,
    });
    expect(body.event_id).toMatch(/^manual-lifecycle:/);
    expect(
      await screen.findByText(/no Schwab order was sent/i),
    ).toBeInTheDocument();
  });

  it("does not render outside an active PAPER Wheel position", () => {
    const { container } = render(
      <StrategyLifecycleControls
        {...base}
        currentState="READY_FOR_PUT"
        executionMode="SHADOW"
      />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});
