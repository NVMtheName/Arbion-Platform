import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
const redirect = vi.hoisted(() =>
  vi.fn((path: string) => {
    throw new Error(`redirect:${path}`);
  }),
);
vi.mock("next/headers", () => ({
  cookies: async () => ({ toString: () => "session=synthetic" }),
}));
vi.mock("next/navigation", () => ({ redirect }));
vi.mock("../app-page-header", () => ({
  AppPageHeader: () => <header>Shared navigation</header>,
}));
import PersonalExecutionPage from "./page";
import { context as commissioningContext } from "./commissioning.test-fixtures";
const response = (value: unknown, status = 200) =>
  new Response(JSON.stringify(value), { status });
beforeEach(() => {
  vi.stubGlobal("fetch", vi.fn());
  vi.stubGlobal("BroadcastChannel", undefined);
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

describe("default-disconnected authenticated execution page", () => {
  it("loads the two private contexts and no provider/order request", async () => {
    vi.mocked(fetch).mockResolvedValue(response({ available: false }));
    render(await PersonalExecutionPage());
    expect(
      screen.getByRole("heading", {
        name: "Personal execution is not enabled",
      }),
    ).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(fetch).toHaveBeenCalledWith(
      expect.stringMatching(/\/api\/personal-execution\/context$/),
      {
        headers: { cookie: "session=synthetic", Accept: "application/json" },
        cache: "no-store",
        redirect: "error",
        signal: expect.any(AbortSignal),
      },
    );
    expect(
      screen.queryByRole("button", { name: "Prepare order" }),
    ).not.toBeInTheDocument();
  });
  it("redirects an unauthenticated session to sign-in", async () => {
    vi.mocked(fetch).mockResolvedValue(response({}, 401));
    await expect(PersonalExecutionPage()).rejects.toThrow("redirect:/login");
    expect(redirect).toHaveBeenCalledOnce();
  });
  it.each([
    response({
      available: true,
      product_id: "BTC-USD",
      session_binding: "a".repeat(64),
    }),
    response({ available: false, account_label: "private" }),
    response({ error: "private" }, 503),
  ])(
    "keeps controls unavailable on malformed or failed upstream context %#",
    async (value) => {
      vi.mocked(fetch).mockResolvedValue(value.clone());
      render(await PersonalExecutionPage());
      expect(
        screen.getByRole("heading", {
          name: "Personal execution is not enabled",
        }),
      ).toBeInTheDocument();
      expect(screen.queryByText("private")).not.toBeInTheDocument();
      expect(fetch).toHaveBeenCalledTimes(2);
    },
  );
  it("mounts the exact available scope without provider or order I/O", async () => {
    vi.mocked(fetch).mockResolvedValue(
      response({
        available: true,
        product_id: "BTC-USD",
        account_label: "Isolated portfolio",
        session_binding: "a".repeat(64),
      }),
    );
    render(await PersonalExecutionPage());
    expect(
      screen.getByRole("button", { name: "Prepare order" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/Isolated portfolio · BTC-USD/),
    ).toBeInTheDocument();
    expect(fetch).toHaveBeenCalledTimes(2);
  });
  it("redirects when the commissioning context detects an expired session", async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(response({ available: false }))
      .mockResolvedValueOnce(response({}, 401));
    await expect(PersonalExecutionPage()).rejects.toThrow("redirect:/login");
    expect(redirect).toHaveBeenCalledOnce();
  });
  it("mounts available setup independently without activating order execution", async () => {
    vi.mocked(fetch)
      .mockResolvedValueOnce(response({ available: false }))
      .mockResolvedValueOnce(response(commissioningContext));
    render(await PersonalExecutionPage());
    expect(
      screen.getByRole("heading", { name: "Review personal Coinbase pilot" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Save exact pilot setup" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Prepare order" }),
    ).not.toBeInTheDocument();
    expect(fetch).toHaveBeenCalledTimes(2);
  });
});
