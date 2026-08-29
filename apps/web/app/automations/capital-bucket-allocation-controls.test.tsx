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

import { CapitalBucketAllocationControls } from "./capital-bucket-allocation-controls";

const base = {
  bucketId: "bucket-1",
  financialAccountId: "account-1",
  name: "Paper Test",
  allocationType: "FIXED_AMOUNT",
  allocationValue: "1.0000000000",
  executionMode: "PAPER",
  currency: "USD",
  protectedAmount: "0.0000000000",
  isReserve: false,
  status: "ACTIVE",
  hasActiveInstance: true,
};

describe("CapitalBucketAllocationControls", () => {
  afterEach(() => {
    cleanup();
    navigation.refresh.mockReset();
    vi.unstubAllGlobals();
  });

  it("updates only the existing capital policy without changing execution", async () => {
    const fetchMock = vi.fn(async () => ({ ok: true }));
    vi.stubGlobal("fetch", fetchMock);
    render(
      <CapitalBucketAllocationControls {...base} hasActiveInstance={false} />,
    );

    fireEvent.change(screen.getByLabelText(/fixed capital allocation/i), {
      target: { value: "1000" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: /update capital guardrail/i }),
    );

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    expect(fetchMock).toHaveBeenCalledWith("/api/capital-buckets/bucket-1", {
      method: "PATCH",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({
        financial_account_id: "account-1",
        name: "Paper Test",
        allocation_type: "FIXED_AMOUNT",
        allocation_value: "1000",
        currency: "USD",
        protected_amount: "0.0000000000",
        is_reserve: false,
      }),
    });
    expect(
      await screen.findByText(/changed only arbion's non-live risk limit/i),
    ).toBeInTheDocument();
    expect(screen.getByText(/not deposited cash/i)).toBeInTheDocument();
    expect(
      screen.getByText(/Paper starting cash is isolated/i),
    ).toBeInTheDocument();
    expect(navigation.refresh).toHaveBeenCalledTimes(1);
  });

  it("freezes the policy while a non-live instance holds its reservation", () => {
    render(<CapitalBucketAllocationControls {...base} />);

    expect(
      screen.getByRole("button", { name: /update capital guardrail/i }),
    ).toBeDisabled();
    expect(
      screen.getByText(/locked while its non-live instance/i),
    ).toBeInTheDocument();
  });

  it("does not allow editing a reserve bucket", () => {
    render(<CapitalBucketAllocationControls {...base} isReserve />);
    expect(
      screen.getByRole("button", { name: /update capital guardrail/i }),
    ).toBeDisabled();
  });
});
