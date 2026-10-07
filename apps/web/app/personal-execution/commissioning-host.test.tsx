import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { StrictMode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CommissioningClient } from "./commissioning-client";
import type { CommissioningContext } from "./commissioning-contract";
const captured = vi.hoisted(() => ({ clients: [] as CommissioningClient[] }));
vi.mock("./commissioning", () => ({
  CommissioningWorkspace: ({ client }: { client: CommissioningClient }) => {
    captured.clients.push(client);
    return <button>Test setup action</button>;
  },
}));
import { CommissioningHost } from "./commissioning-host";
import {
  executionLogoutEvent,
  executionSessionChannel,
} from "./session-events";
import {
  context,
  review,
  receipt,
  response,
} from "./commissioning.test-fixtures";
const latest = () => captured.clients.at(-1)!;
beforeEach(() => {
  captured.clients = [];
  vi.stubGlobal("fetch", vi.fn());
  vi.stubGlobal("BroadcastChannel", undefined);
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("commissioning authenticated session lifetime", () => {
  it("mounts exact context without I/O, includingStrictMode", () => {
    render(
      <StrictMode>
        <CommissioningHost context={context} />
      </StrictMode>,
    );
    expect(
      screen.getByRole("button", { name: "Test setup action" }),
    ).toBeInTheDocument();
    expect(fetch).not.toHaveBeenCalled();
  });
  it.each([
    { available: false },
    { ...context, account_id: "private" },
    { ...context, session_binding: "bad" },
  ])("hides unavailable or malformed context %#", (value) => {
    render(<CommissioningHost context={value as CommissioningContext} />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(captured.clients).toHaveLength(0);
    expect(fetch).not.toHaveBeenCalled();
  });
  it.each(["pagehide", executionLogoutEvent])(
    "closes permanently on%s and ignoreslate response",
    async (event) => {
      let resolve!: (value: Response) => void;
      vi.mocked(fetch).mockImplementation(
        () =>
          new Promise((done) => {
            resolve = done;
          }),
      );
      render(<CommissioningHost context={context} />);
      const old = latest(),
        pending = old.prepare();
      act(() => window.dispatchEvent(new Event(event)));
      resolve(response({ receipt }));
      await expect(pending).rejects.toMatchObject({
        code: "execution_session_changed",
      });
      expect(
        screen.getByRole("heading", { name: "Setup session closed" }),
      ).toBeInTheDocument();
      await expect(old.readReceipt()).rejects.toMatchObject({
        code: "execution_session_changed",
      });
      expect(fetch).toHaveBeenCalledOnce();
    },
  );
  it("does not revive after scope replacement or replacementback", async () => {
    const view = render(<CommissioningHost context={context} />),
      old = latest();
    view.rerender(
      <CommissioningHost
        context={{
          available: true,
          session_binding: "c".repeat(64),
          review: { ...review, model_id: "other" },
        }}
      />,
    );
    view.rerender(<CommissioningHost context={context} />);
    expect(
      screen.getByRole("heading", { name: "Setup session closed" }),
    ).toBeInTheDocument();
    await expect(old.readReceipt()).rejects.toMatchObject({
      code: "execution_session_changed",
    });
    expect(fetch).not.toHaveBeenCalled();
  });
  it("hiding the page closes controls", () => {
    render(<CommissioningHost context={context} />);
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
    act(() => document.dispatchEvent(new Event("visibilitychange")));
    expect(
      screen.getByRole("heading", { name: "Setup session closed" }),
    ).toBeInTheDocument();
    expect(fetch).not.toHaveBeenCalled();
  });
  it("bfcache restoration closes controls", () => {
    render(<CommissioningHost context={context} />);
    act(() =>
      window.dispatchEvent(
        new PageTransitionEvent("pageshow", { persisted: true }),
      ),
    );
    expect(
      screen.getByRole("heading", { name: "Setup session closed" }),
    ).toBeInTheDocument();
    expect(fetch).not.toHaveBeenCalled();
  });
  it("cross-tab logout closes controls and releases its channel", () => {
    const close = vi.fn();
    let channel!: {
      onmessage: ((event: MessageEvent) => void) | null;
      close: () => void;
    };
    const constructor = vi.fn(function () {
      channel = { onmessage: null, close };
      return channel;
    });
    vi.stubGlobal("BroadcastChannel", constructor);
    const view = render(<CommissioningHost context={context} />);
    expect(constructor).toHaveBeenCalledWith(executionSessionChannel);
    act(() =>
      channel.onmessage?.(new MessageEvent("message", { data: "logout" })),
    );
    expect(
      screen.getByRole("heading", { name: "Setup session closed" }),
    ).toBeInTheDocument();
    view.unmount();
    expect(close).toHaveBeenCalledOnce();
    expect(fetch).not.toHaveBeenCalled();
  });
  it("only rechecks context on focus and closes for a changed session", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      response({ ...context, session_binding: "d".repeat(64) }),
    );
    render(<CommissioningHost context={context} />);
    act(() => {
      window.dispatchEvent(new Event("focus"));
      window.dispatchEvent(new Event("focus"));
    });
    await waitFor(() =>
      expect(
        screen.getByRole("heading", { name: "Setup session closed" }),
      ).toBeInTheDocument(),
    );
    expect(fetch).toHaveBeenCalledOnce();
    expect(fetch).toHaveBeenCalledWith(
      "/api/personal-execution/commissioning/context",
      expect.objectContaining({
        cache: "no-store",
        credentials: "same-origin",
        redirect: "error",
      }),
    );
  });
  it.each([401, 403, 503])(
    "closes for contextHTTP%s without reflectingbody",
    async (status) => {
      vi.mocked(fetch).mockResolvedValueOnce(
        response({ error: "private" }, status),
      );
      render(<CommissioningHost context={context} />);
      act(() => window.dispatchEvent(new Event("focus")));
      await screen.findByRole("heading", { name: "Setup session closed" });
      expect(screen.queryByText("private")).not.toBeInTheDocument();
    },
  );
});
