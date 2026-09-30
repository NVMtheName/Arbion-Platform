import { describe, expect, it } from "vitest";
import {
  parseOwnerExecutionContext,
  readOwnerExecutionContext,
} from "./context";

const available = {
  available: true,
  product_id: "BTC-USD",
  account_label: "Isolated portfolio",
  session_binding: "b".repeat(64),
};

describe("trusted owner execution context", () => {
  it("accepts only the exact disabled or safe fixed scope contract", () => {
    expect(parseOwnerExecutionContext({ available: false })).toEqual({
      available: false,
    });
    expect(parseOwnerExecutionContext(available)).toEqual(available);
  });
  it.each([
    null,
    [],
    {},
    { available: 1 },
    { available: false, product_id: "BTC-USD" },
    { ...available, session_binding: "" },
    { ...available, session_binding: "B".repeat(64) },
    { ...available, product_id: "USD-USD" },
    { ...available, product_id: "ETH-EUR" },
    { ...available, product_id: "1-USD" },
    { ...available, product_id: "A".repeat(17) + "-USD" },
    { ...available, account_label: " " },
    { ...available, account_label: "private\nlabel" },
    { ...available, account_label: "a".repeat(121) },
    { ...available, owner_id: "private" },
    Object.create(available),
  ])(
    "rejects malformed, private, widened or conflicting context %#",
    (value) => {
      expect(() => parseOwnerExecutionContext(value)).toThrow();
    },
  );
  it("rejects duplicate members, non-200 responses and response redirects", async () => {
    await expect(
      readOwnerExecutionContext(
        new Response('{"available":true,"available":false}'),
      ),
    ).rejects.toThrow();
    await expect(
      readOwnerExecutionContext(
        new Response('{"available":false}', { status: 201 }),
      ),
    ).rejects.toThrow();
    const redirected = new Response('{"available":false}');
    Object.defineProperty(redirected, "redirected", { value: true });
    await expect(readOwnerExecutionContext(redirected)).rejects.toThrow();
  });
});
