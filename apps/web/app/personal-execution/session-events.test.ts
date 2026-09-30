import { afterEach, describe, expect, it, vi } from "vitest";
import { executionLogoutEvent, notifyExecutionLogout } from "./session-events";

afterEach(() => vi.unstubAllGlobals());

describe("execution logout notification", () => {
  it.each(["construct", "post", "close"])(
    "cannot block logout when BroadcastChannel fails at %s",
    (stage) => {
      const listener = vi.fn();
      window.addEventListener(executionLogoutEvent, listener);
      vi.stubGlobal(
        "BroadcastChannel",
        class {
          constructor() {
            if (stage === "construct") throw new Error("denied");
          }
          postMessage() {
            if (stage === "post") throw new Error("denied");
          }
          close() {
            if (stage === "close") throw new Error("denied");
          }
        },
      );
      try {
        expect(notifyExecutionLogout).not.toThrow();
        expect(listener).toHaveBeenCalledOnce();
      } finally {
        window.removeEventListener(executionLogoutEvent, listener);
      }
    },
  );
});
