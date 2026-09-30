import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { StrictMode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { OwnerExecutionClient } from "./client";
import type { OwnerExecutionContext } from "./context";

const captured = vi.hoisted(() => ({ clients: [] as OwnerExecutionClient[] }));
vi.mock("./owner-execution", () => ({
  OwnerExecutionWorkspace: ({
    client,
    productID,
    accountLabel,
  }: {
    client: OwnerExecutionClient;
    productID: string;
    accountLabel: string;
  }) => {
    captured.clients.push(client);
    return (
      <section aria-label="Execution workspace">
        {accountLabel} · {productID}
        <button>Prepare order</button>
      </section>
    );
  },
}));
import { OwnerExecutionHost } from "./owner-host";
import {
  executionLogoutEvent,
  executionSessionChannel,
  notifyExecutionLogout,
} from "./session-events";

const id = "11111111-1111-4111-8111-111111111111";
const context: OwnerExecutionContext = {
  available: true,
  product_id: "BTC-USD",
  account_label: "Isolated portfolio",
  session_binding: "a".repeat(64),
};
const response = (value: unknown, status = 200) =>
  new Response(JSON.stringify(value), { status });
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

describe("authenticated execution workspace lifetime", () => {
  it("mounts the trusted fixed scope without any network request, including StrictMode", async () => {
    render(
      <StrictMode>
        <OwnerExecutionHost context={context} />
      </StrictMode>,
    );
    expect(
      screen.getByRole("region", { name: "Execution workspace" }),
    ).toHaveTextContent("Isolated portfolio · BTC-USD");
    expect(fetch).not.toHaveBeenCalled();
    const fetchMock = vi.mocked(fetch).mockResolvedValue(response({}, 401));
    await act(async () => {
      await expect(latest().get(id)).rejects.toMatchObject({
        code: "execution_session_changed",
      });
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(
      screen.queryByRole("button", { name: "Prepare order" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Execution session closed" }),
    ).toBeInTheDocument();
  });

  it.each([
    { available: false },
    { ...context, session_binding: "bad" },
    { ...context, owner_id: "private" },
  ])("never mounts an unavailable or malformed context %#", (value) => {
    render(<OwnerExecutionHost context={value as OwnerExecutionContext} />);
    expect(
      screen.getByRole("heading", {
        name: "Personal execution is not enabled",
      }),
    ).toBeInTheDocument();
    expect(captured.clients).toHaveLength(0);
    expect(fetch).not.toHaveBeenCalled();
  });

  it("never revives A after A -> B -> A context changes", async () => {
    const view = render(<OwnerExecutionHost context={context} />);
    const old = latest();
    view.rerender(
      <OwnerExecutionHost
        context={{ ...context, session_binding: "b".repeat(64) }}
      />,
    );
    view.rerender(<OwnerExecutionHost context={context} />);
    expect(
      screen.getByRole("heading", { name: "Execution session closed" }),
    ).toBeInTheDocument();
    await expect(old.get(id)).rejects.toMatchObject({
      code: "execution_session_changed",
    });
    expect(fetch).not.toHaveBeenCalled();
  });

  it.each(["pagehide", executionLogoutEvent])(
    "closes permanently on %s and rejects late old-session results",
    async (event) => {
      let resolve!: (value: Response) => void;
      vi.mocked(fetch).mockImplementation(
        () =>
          new Promise<Response>((done) => {
            resolve = done;
          }),
      );
      render(<OwnerExecutionHost context={context} />);
      const old = latest();
      const pending = old.get(id);
      const outcome = expect(pending).rejects.toMatchObject({
        code: "execution_session_changed",
      });
      act(() => window.dispatchEvent(new Event(event)));
      resolve(response({ private: "never display" }));
      await outcome;
      expect(
        screen.getByRole("heading", { name: "Execution session closed" }),
      ).toBeInTheDocument();
      expect(screen.queryByText("never display")).not.toBeInTheDocument();
      act(() => window.dispatchEvent(new Event("focus")));
      await expect(old.get(id)).rejects.toMatchObject({
        code: "execution_session_changed",
      });
      expect(fetch).toHaveBeenCalledTimes(1);
    },
  );

  it("hiding then showing does not reopen or recheck a closed workspace", async () => {
    render(<OwnerExecutionHost context={context} />);
    const old = latest();
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
    act(() => document.dispatchEvent(new Event("visibilitychange")));
    vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
    act(() => document.dispatchEvent(new Event("visibilitychange")));
    expect(
      screen.queryByRole("button", { name: "Prepare order" }),
    ).not.toBeInTheDocument();
    await expect(old.get(id)).rejects.toMatchObject({
      code: "execution_session_changed",
    });
    expect(fetch).not.toHaveBeenCalled();
  });

  it("closes a bfcache-restored workspace", () => {
    render(<OwnerExecutionHost context={context} />);
    act(() =>
      window.dispatchEvent(
        new PageTransitionEvent("pageshow", { persisted: true }),
      ),
    );
    expect(
      screen.getByRole("heading", { name: "Execution session closed" }),
    ).toBeInTheDocument();
    expect(fetch).not.toHaveBeenCalled();
  });

  it("rechecks only context on focus, with one pending request and no repeated order call", async () => {
    let resolve!: (value: Response) => void;
    vi.mocked(fetch).mockImplementation(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    );
    render(<OwnerExecutionHost context={context} />);
    act(() => {
      window.dispatchEvent(new Event("focus"));
      window.dispatchEvent(new Event("focus"));
    });
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch).toHaveBeenCalledWith("/api/personal-execution/context", {
      credentials: "same-origin",
      cache: "no-store",
      redirect: "error",
      headers: { Accept: "application/json" },
      signal: expect.any(AbortSignal),
    });
    await act(async () => {
      resolve(response(context));
    });
    expect(
      screen.getByRole("button", { name: "Prepare order" }),
    ).toBeInTheDocument();
    vi.mocked(fetch).mockResolvedValue(
      response({ ...context, session_binding: "c".repeat(64) }),
    );
    act(() => window.dispatchEvent(new Event("focus")));
    await waitFor(() =>
      expect(
        screen.getByRole("heading", { name: "Execution session closed" }),
      ).toBeInTheDocument(),
    );
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it.each([401, 403, 503])(
    "closes on context HTTP %s without reflecting response text",
    async (status) => {
      vi.mocked(fetch).mockResolvedValue(
        response({ error: "private upstream text" }, status),
      );
      render(<OwnerExecutionHost context={context} />);
      act(() => window.dispatchEvent(new Event("focus")));
      await waitFor(() =>
        expect(
          screen.getByRole("heading", { name: "Execution session closed" }),
        ).toBeInTheDocument(),
      );
      expect(
        screen.queryByText("private upstream text"),
      ).not.toBeInTheDocument();
    },
  );

  it("invalidates on unmount and ignores late context completion", async () => {
    let resolve!: (value: Response) => void;
    vi.mocked(fetch).mockImplementation(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    );
    const view = render(<OwnerExecutionHost context={context} />);
    const old = latest();
    act(() => window.dispatchEvent(new Event("focus")));
    view.unmount();
    expect(vi.mocked(fetch).mock.calls[0][1]?.signal?.aborted).toBe(true);
    await act(async () => {
      resolve(response(context));
    });
    await expect(old.get(id)).rejects.toMatchObject({
      code: "execution_session_changed",
    });
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("listens to cross-tab logout and closes the channel on unmount", () => {
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
    const view = render(<OwnerExecutionHost context={context} />);
    expect(constructor).toHaveBeenCalledWith(executionSessionChannel);
    act(() =>
      channel.onmessage?.(new MessageEvent("message", { data: "logout" })),
    );
    expect(
      screen.getByRole("heading", { name: "Execution session closed" }),
    ).toBeInTheDocument();
    view.unmount();
    expect(close).toHaveBeenCalledOnce();
  });

  it("fails closed if cross-tab notification cannot be initialized; logout itself still proceeds", () => {
    vi.stubGlobal(
      "BroadcastChannel",
      class {
        constructor() {
          throw new Error("denied");
        }
      },
    );
    render(<OwnerExecutionHost context={context} />);
    expect(
      screen.getByRole("heading", { name: "Execution session closed" }),
    ).toBeInTheDocument();
    expect(notifyExecutionLogout).not.toThrow();
    expect(fetch).not.toHaveBeenCalled();
  });
});
