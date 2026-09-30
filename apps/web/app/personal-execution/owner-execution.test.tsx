import { StrictMode } from "react";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { OwnerExecutionClient, OwnerOrder } from "./client";
import { OwnerExecutionWorkspace } from "./owner-execution";

const id = "11111111-1111-4111-8111-111111111111";
const otherID = "22222222-2222-4222-8222-222222222222";
const evidenceID = "33333333-3333-4333-8333-333333333333";
const digest = "a".repeat(64);
const terms = {
  side: "BUY" as const,
  base_size: "0.001000000000000001",
  limit_price: "30000.000000000000000000",
  fee_allowance_usd: "0.100000000000000001",
  maximum_debit_usd: "30.100000000000100000",
};
const inputs = [
  "Base quantity",
  "Limit price (USD)",
  "Fee allowance (USD)",
  "Maximum debit (USD)",
];
const fields = [
  "base_size",
  "limit_price",
  "fee_allowance_usd",
  "maximum_debit_usd",
] as const;

function order(
  state: OwnerOrder["state"],
  extra: Partial<OwnerOrder> = {},
): OwnerOrder {
  const claimed = !["PREPARED", "APPROVED", "NOT_SENT"].includes(state);
  const terminal =
    state === "AWAITING_ACCOUNT_SETTLEMENT" || state === "SETTLED";
  const filled = state === "PARTIALLY_FILLED" || terminal;
  return {
    id,
    request_digest: digest,
    product_id: "BTC-USD",
    ...terms,
    created_at: "2026-09-30T12:00:00Z",
    state,
    summary: "Untrusted summary must not drive controls.",
    approval_status: state === "PREPARED" ? "NONE" : "RECORDED",
    ...(state !== "PREPARED"
      ? { approval_expires_at: "2099-01-01T00:00:00Z" }
      : {}),
    cancellation_status: "NONE",
    account_held: claimed && !terminal,
    capital_held: claimed && state !== "SETTLED",
    account_blocked: false,
    fill_count: terminal ? 2 : filled ? 1 : 0,
    base_filled: terminal ? "0.0008" : filled ? "0.0004" : "0",
    gross_usd: terminal ? "24" : filled ? "12" : "0",
    fee_usd: terminal ? "0.08" : filled ? "0.04" : "0",
    ...(terminal ? { terminal_status: "CANCELLED" as const } : {}),
    ...(state === "SETTLED"
      ? {
          accounting: {
            opening_cash_usd: "1000.1",
            opening_base: "1",
            closing_cash_usd: "976.02",
            closing_base: "1.0008",
            recorded_at: "2026-09-30T12:01:00Z",
          },
        }
      : {}),
    ...extra,
  };
}

function clientFixture() {
  return {
    prepare: vi
      .fn<OwnerExecutionClient["prepare"]>()
      .mockResolvedValue(order("PREPARED")),
    get: vi
      .fn<OwnerExecutionClient["get"]>()
      .mockResolvedValue(order("PREPARED")),
    approve: vi
      .fn<OwnerExecutionClient["approve"]>()
      .mockResolvedValue(order("APPROVED")),
    preflight: vi
      .fn<OwnerExecutionClient["preflight"]>()
      .mockResolvedValue({ order: order("APPROVED"), evidence_id: evidenceID }),
    send: vi
      .fn<OwnerExecutionClient["send"]>()
      .mockResolvedValue(order("BROKER_ACKNOWLEDGED")),
    revoke: vi
      .fn<OwnerExecutionClient["revoke"]>()
      .mockResolvedValue(order("PREPARED", { approval_status: "REVOKED" })),
    recover: vi
      .fn<OwnerExecutionClient["recover"]>()
      .mockResolvedValue(order("BROKER_ACKNOWLEDGED")),
    reconcile: vi
      .fn<OwnerExecutionClient["reconcile"]>()
      .mockResolvedValue(order("PARTIALLY_FILLED")),
    cancel: vi
      .fn<OwnerExecutionClient["cancel"]>()
      .mockResolvedValue(
        order("PARTIALLY_FILLED", { cancellation_status: "ACCEPTED" }),
      ),
    settle: vi
      .fn<OwnerExecutionClient["settle"]>()
      .mockResolvedValue(order("SETTLED", { cancellation_status: "ACCEPTED" })),
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function workspace(
  client: OwnerExecutionClient,
  productID = "BTC-USD",
  accountLabel = "Isolated personal portfolio",
) {
  return (
    <OwnerExecutionWorkspace
      client={client}
      productID={productID}
      accountLabel={accountLabel}
    />
  );
}

async function prepare() {
  fields.forEach((field, i) =>
    fireEvent.change(screen.getByLabelText(inputs[i]), {
      target: { value: terms[field] },
    }),
  );
  fireEvent.click(screen.getByRole("button", { name: "Prepare order" }));
  await screen.findByRole("heading", { name: "PREPARED" });
}

async function approve() {
  fireEvent.change(screen.getByLabelText("Execution MFA code"), {
    target: { value: "123456" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Approve exact terms" }));
  await screen.findByRole("heading", { name: "APPROVED" });
}

async function readyToSend() {
  await prepare();
  await approve();
  fireEvent.click(
    screen.getByRole("button", { name: "Check current preflight" }),
  );
  await screen.findByRole("button", { name: "Send order once" });
}

async function load(
  state: OwnerOrder["state"],
  client: ReturnType<typeof clientFixture>,
  extra: Partial<OwnerOrder> = {},
) {
  client.get.mockResolvedValue(order(state, extra));
  fireEvent.change(screen.getByLabelText("Saved order ID"), {
    target: { value: id },
  });
  fireEvent.click(screen.getByRole("button", { name: "Read saved status" }));
  await screen.findByRole("heading", { name: state.replaceAll("_", " ") });
}

describe("OwnerExecutionWorkspace inert acceptance", () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("performs no mount or StrictMode calls and loads a saved ID without automatic sending", async () => {
    const client = clientFixture();
    const view = render(<StrictMode>{workspace(client)}</StrictMode>);
    Object.values(client).forEach((method) =>
      expect(method).not.toHaveBeenCalled(),
    );
    await load("APPROVED", client);
    expect(client.get).toHaveBeenCalledExactlyOnceWith(id);
    expect(client.send).not.toHaveBeenCalled();
    expect(client.preflight).not.toHaveBeenCalled();
    expect(
      screen.queryByRole("button", { name: "Send order once" }),
    ).not.toBeInTheDocument();
    expect(screen.getByLabelText("Saved order ID")).toHaveAttribute("readonly");
    view.unmount();
    expect(client.send).not.toHaveBeenCalled();
  });

  it("finishes the explicit prepare, approve, preflight, send, fill, cancel and settlement workflow", async () => {
    const client = clientFixture();
    client.reconcile
      .mockResolvedValueOnce(order("PARTIALLY_FILLED"))
      .mockResolvedValueOnce(
        order("AWAITING_ACCOUNT_SETTLEMENT", {
          cancellation_status: "ACCEPTED",
        }),
      );
    render(workspace(client));
    await prepare();
    expect(client.prepare).toHaveBeenCalledWith({
      ...terms,
      request_key: expect.stringMatching(/^[a-f0-9-]{36}$/),
    });
    fields.forEach((field) =>
      expect(
        screen.getByText(terms[field], { selector: "dd" }),
      ).toBeInTheDocument(),
    );
    await approve();
    expect(client.approve).toHaveBeenCalledExactlyOnceWith(id, {
      expected_digest: digest,
      mfa_code: "123456",
    });
    expect(
      screen.queryByLabelText("Execution MFA code"),
    ).not.toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "Check current preflight" }),
    );
    const send = await screen.findByRole("button", { name: "Send order once" });
    expect(send).toBeDisabled();
    expect(client.send).not.toHaveBeenCalled();
    fireEvent.click(
      screen.getByRole("checkbox", {
        name: /I confirm one price-bounded order/,
      }),
    );
    fireEvent.click(send);
    await screen.findByRole("heading", { name: "BROKER ACKNOWLEDGED" });
    expect(client.send).toHaveBeenCalledExactlyOnceWith(id, {
      evidence_id: evidenceID,
    });
    expect(client.reconcile).not.toHaveBeenCalled();
    fireEvent.click(
      screen.getByRole("button", { name: "Reconcile fills and status" }),
    );
    await screen.findByRole("heading", { name: "PARTIALLY FILLED" });
    expect(screen.getByText("0.0004", { selector: "dd" })).toBeInTheDocument();
    const cancel = screen.getByRole("button", {
      name: "Request cancellation once",
    });
    expect(cancel).toBeDisabled();
    fireEvent.click(
      screen.getByRole("checkbox", {
        name: /Request cancellation of only this original order/,
      }),
    );
    fireEvent.click(cancel);
    await screen.findByText(/Cancellation: ACCEPTED/);
    expect(client.cancel).toHaveBeenCalledExactlyOnceWith(id);
    expect(
      screen.queryByRole("button", { name: "Reconcile cash and position" }),
    ).not.toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "Reconcile fills and status" }),
    );
    await screen.findByRole("heading", { name: "AWAITING ACCOUNT SETTLEMENT" });
    expect(client.settle).not.toHaveBeenCalled();
    fireEvent.click(
      screen.getByRole("button", { name: "Reconcile cash and position" }),
    );
    await screen.findByRole("heading", { name: "SETTLED" });
    fireEvent.click(screen.getByText("Exact saved evidence"));
    expect(screen.getByText("976.02", { selector: "dd" })).toBeVisible();
    expect(screen.getByText("1.0008", { selector: "dd" })).toBeVisible();
    expect(client.preflight).toHaveBeenCalledExactlyOnceWith(id);
    expect(client.settle).toHaveBeenCalledExactlyOnceWith(id);
    expect(client.send).toHaveBeenCalledTimes(1);
    expect(client.cancel).toHaveBeenCalledTimes(1);
  });

  it("never retries a lost send even after a stale APPROVED saved snapshot", async () => {
    const client = clientFixture();
    client.send.mockRejectedValue(
      new Error("secret broker token lost response"),
    );
    client.get.mockResolvedValue(order("APPROVED"));
    render(workspace(client));
    await readyToSend();
    fireEvent.click(
      screen.getByRole("checkbox", {
        name: /I confirm one price-bounded order/,
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Send order once" }));
    await screen.findByRole("alert");
    expect(screen.queryByText(/secret broker token/)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Read saved status" }));
    await screen.findByRole("button", { name: "Recover original submission" });
    expect(
      screen.queryByRole("button", { name: "Send order once" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Check current preflight" }),
    ).not.toBeInTheDocument();
    expect(client.send).toHaveBeenCalledTimes(1);
    expect(client.recover).not.toHaveBeenCalled();
  });

  it("never retries a lost cancellation after a stale NONE cancellation snapshot", async () => {
    const client = clientFixture();
    client.cancel.mockRejectedValue(new Error("secret cancel response"));
    render(workspace(client));
    await load("PARTIALLY_FILLED", client);
    fireEvent.click(
      screen.getByRole("checkbox", {
        name: /Request cancellation of only this original order/,
      }),
    );
    fireEvent.click(
      screen.getByRole("button", { name: "Request cancellation once" }),
    );
    await screen.findByRole("alert");
    fireEvent.click(screen.getByRole("button", { name: "Read saved status" }));
    await screen.findByRole("button", { name: "Reconcile fills and status" });
    expect(
      screen.queryByRole("button", { name: "Request cancellation once" }),
    ).not.toBeInTheDocument();
    expect(client.cancel).toHaveBeenCalledTimes(1);
    expect(client.reconcile).not.toHaveBeenCalled();
  });

  it("admits only one synchronous double-click before a send promise completes", async () => {
    const client = clientFixture();
    const pending = deferred<OwnerOrder>();
    client.send.mockReturnValue(pending.promise);
    render(workspace(client));
    await readyToSend();
    fireEvent.click(
      screen.getByRole("checkbox", {
        name: /I confirm one price-bounded order/,
      }),
    );
    const button = screen.getByRole("button", { name: "Send order once" });
    act(() => {
      fireEvent.click(button);
      fireEvent.click(button);
    });
    expect(client.send).toHaveBeenCalledTimes(1);
    await act(async () => pending.resolve(order("BROKER_ACKNOWLEDGED")));
    expect(
      screen.queryByRole("button", { name: "Send order once" }),
    ).not.toBeInTheDocument();
  });

  it("clears MFA on failure and never renders raw errors or the untrusted summary", async () => {
    const client = clientFixture();
    client.approve.mockRejectedValue(new Error("private-key-secret 123456"));
    render(workspace(client));
    await prepare();
    fireEvent.change(screen.getByLabelText("Execution MFA code"), {
      target: { value: "123456" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Approve exact terms" }),
    );
    expect(await screen.findByRole("alert")).toHaveTextContent(
      /result could not be verified/,
    );
    expect(
      screen.queryByText(/private-key-secret|123456|Untrusted summary/),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Read saved status" }));
    expect(await screen.findByLabelText("Execution MFA code")).toHaveValue("");
    expect(client.approve).toHaveBeenCalledTimes(1);
  });

  it("recovers lost preparation only with the same frozen key and exact terms", async () => {
    const client = clientFixture();
    client.prepare.mockRejectedValueOnce(
      new Error("lost preparation response"),
    );
    render(workspace(client));
    fields.forEach((field, i) =>
      fireEvent.change(screen.getByLabelText(inputs[i]), {
        target: { value: terms[field] },
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Prepare order" }));
    await screen.findByRole("alert");
    inputs.forEach((label) =>
      expect(screen.getByLabelText(label)).toBeDisabled(),
    );
    const first = client.prepare.mock.calls[0][0];
    fireEvent.click(
      screen.getByRole("button", { name: "Recover preparation" }),
    );
    await screen.findByRole("heading", { name: "PREPARED" });
    expect(client.prepare.mock.calls[1][0]).toEqual(first);
    expect(client.send).not.toHaveBeenCalled();
    expect(client.approve).not.toHaveBeenCalled();
  });

  it.each([
    "SUBMISSION_UNKNOWN",
    "REJECTED_HELD",
    "UNAVAILABLE",
    "NOT_SENT",
  ] as const)("offers no unsafe mutation controls for %s", async (state) => {
    const client = clientFixture();
    render(workspace(client));
    await load(state, client);
    for (const name of [
      "Approve exact terms",
      "Check current preflight",
      "Send order once",
      "Request cancellation once",
      "Reconcile cash and position",
    ]) {
      expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
    }
    expect(client.send).not.toHaveBeenCalled();
    expect(client.cancel).not.toHaveBeenCalled();
    expect(client.settle).not.toHaveBeenCalled();
  });

  it.each(["product", "order", "digest"])(
    "blocks a mismatched %s response",
    async (field) => {
      const client = clientFixture();
      const changed =
        field === "product"
          ? { product_id: "ETH-USD" }
          : field === "order"
            ? { id: otherID }
            : { request_digest: "b".repeat(64) };
      client.approve.mockResolvedValue(order("APPROVED", changed));
      render(workspace(client));
      await prepare();
      fireEvent.change(screen.getByLabelText("Execution MFA code"), {
        target: { value: "123456" },
      });
      fireEvent.click(
        screen.getByRole("button", { name: "Approve exact terms" }),
      );
      await screen.findByRole("alert");
      expect(
        screen.getByRole("heading", { name: "PREPARED" }),
      ).toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: "Check current preflight" }),
      ).not.toBeInTheDocument();
      expect(client.send).not.toHaveBeenCalled();
    },
  );

  it.each(["product", "side", ...fields])(
    "refuses an initial preparation response with changed %s",
    async (field) => {
      const client = clientFixture();
      const changed =
        field === "product"
          ? { product_id: "ETH-USD" }
          : field === "side"
            ? { side: "SELL" as const }
            : { [field]: "2" };
      client.prepare.mockResolvedValue(order("PREPARED", changed));
      render(workspace(client));
      fields.forEach((key, i) =>
        fireEvent.change(screen.getByLabelText(inputs[i]), {
          target: { value: terms[key] },
        }),
      );
      fireEvent.click(screen.getByRole("button", { name: "Prepare order" }));
      await screen.findByRole("alert");
      expect(
        screen.queryByRole("heading", { name: "PREPARED" }),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: "Approve exact terms" }),
      ).not.toBeInTheDocument();
      expect(client.prepare).toHaveBeenCalledTimes(1);
      expect(client.approve).not.toHaveBeenCalled();
      expect(client.send).not.toHaveBeenCalled();
    },
  );

  it("keeps invalid preparation editable and sends only corrected valid terms", async () => {
    const client = clientFixture();
    render(workspace(client));
    fields.forEach((field, i) =>
      fireEvent.change(screen.getByLabelText(inputs[i]), {
        target: { value: field === "base_size" ? "0" : terms[field] },
      }),
    );
    fireEvent.click(screen.getByRole("button", { name: "Prepare order" }));
    expect(screen.getByRole("alert")).toHaveTextContent("No request was sent");
    expect(client.prepare).not.toHaveBeenCalled();
    inputs.forEach((label) =>
      expect(screen.getByLabelText(label)).not.toBeDisabled(),
    );
    await prepare();
    expect(client.prepare).toHaveBeenCalledExactlyOnceWith({
      ...terms,
      request_key: expect.stringMatching(/^[a-f0-9-]{36}$/),
    });
  });

  it("refuses an initial saved-order response for another order ID", async () => {
    const client = clientFixture();
    client.get.mockResolvedValue(order("APPROVED", { id: otherID }));
    render(workspace(client));
    fireEvent.change(screen.getByLabelText("Saved order ID"), {
      target: { value: id },
    });
    fireEvent.click(screen.getByRole("button", { name: "Read saved status" }));
    await screen.findByRole("alert");
    expect(client.get).toHaveBeenCalledExactlyOnceWith(id);
    expect(
      screen.queryByRole("heading", { name: "APPROVED" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Check current preflight" }),
    ).not.toBeInTheDocument();
    expect(client.send).not.toHaveBeenCalled();
  });

  it.each([
    ["product", "ready"],
    ["account label", "ready"],
    ["client", "ready"],
    ["product", "pending"],
    ["account label", "pending"],
    ["client", "pending"],
  ])(
    "never restores %s scope authority after A-to-B-to-A with %s preflight",
    async (scope, phase) => {
      const client = clientFixture();
      const otherClient = clientFixture();
      const pending =
        deferred<Awaited<ReturnType<OwnerExecutionClient["preflight"]>>>();
      if (phase === "pending")
        client.preflight.mockReturnValue(pending.promise);
      const view = render(workspace(client));
      await prepare();
      await approve();
      fireEvent.click(
        screen.getByRole("button", { name: "Check current preflight" }),
      );
      expect(client.preflight).toHaveBeenCalledTimes(1);
      if (phase === "ready") {
        await screen.findByRole("button", { name: "Send order once" });
        fireEvent.click(
          screen.getByRole("checkbox", {
            name: /I confirm one price-bounded order/,
          }),
        );
        expect(
          screen.getByRole("button", { name: "Send order once" }),
        ).toBeEnabled();
      }
      view.rerender(
        workspace(
          scope === "client" ? otherClient : client,
          scope === "product" ? "ETH-USD" : "BTC-USD",
          scope === "account label" ? "Another portfolio" : undefined,
        ),
      );
      view.rerender(workspace(client));
      if (phase === "pending") {
        await act(async () =>
          pending.resolve({
            order: order("APPROVED"),
            evidence_id: evidenceID,
          }),
        );
      }
      expect(screen.getByRole("alert")).toHaveTextContent(
        /scope is unavailable or changed/,
      );
      expect(screen.queryByRole("button")).not.toBeInTheDocument();
      expect(client.send).not.toHaveBeenCalled();
      expect(client.preflight).toHaveBeenCalledTimes(1);
      Object.values(otherClient).forEach((method) =>
        expect(method).not.toHaveBeenCalled(),
      );
    },
  );

  it.each(["scope change", "unmount"])(
    "discards a late preflight after %s",
    async (mode) => {
      const client = clientFixture();
      const pending =
        deferred<Awaited<ReturnType<OwnerExecutionClient["preflight"]>>>();
      client.preflight.mockReturnValue(pending.promise);
      const view = render(workspace(client));
      await prepare();
      await approve();
      fireEvent.click(
        screen.getByRole("button", { name: "Check current preflight" }),
      );
      expect(client.preflight).toHaveBeenCalledTimes(1);
      if (mode === "scope change") view.rerender(workspace(client, "ETH-USD"));
      else view.unmount();
      await act(async () =>
        pending.resolve({ order: order("APPROVED"), evidence_id: evidenceID }),
      );
      if (mode === "scope change")
        expect(screen.getByRole("alert")).toHaveTextContent(
          /scope is unavailable or changed/,
        );
      else {
        render(workspace(client));
        await load("APPROVED", client);
      }
      expect(
        screen.queryByRole("button", { name: "Send order once" }),
      ).not.toBeInTheDocument();
      expect(client.send).not.toHaveBeenCalled();
    },
  );

  it("rechecks approval expiration at click time even if the old send control remains rendered", async () => {
    const client = clientFixture();
    const now = Date.parse("2026-09-30T12:00:00Z");
    const clock = vi.spyOn(Date, "now").mockReturnValue(now);
    const approved = order("APPROVED", {
      approval_expires_at: new Date(now + 60_000).toISOString(),
    });
    client.approve.mockResolvedValue(approved);
    client.preflight.mockResolvedValue({
      order: approved,
      evidence_id: evidenceID,
    });
    render(workspace(client));
    await readyToSend();
    fireEvent.click(
      screen.getByRole("checkbox", {
        name: /I confirm one price-bounded order/,
      }),
    );
    const button = screen.getByRole("button", { name: "Send order once" });
    clock.mockReturnValue(now + 60_000);
    fireEvent.click(button);
    expect(client.send).not.toHaveBeenCalled();
    await waitFor(() => expect(client.preflight).toHaveBeenCalledTimes(1));
  });
});
