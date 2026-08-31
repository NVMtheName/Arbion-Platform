import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const navigation = vi.hoisted(() => ({
  push: vi.fn(),
  refresh: vi.fn(),
  pathname: "/accounts",
}));
vi.mock("next/navigation", () => ({
  useRouter: () => navigation,
  usePathname: () => navigation.pathname,
}));

import { AuthForm } from "./auth-form";
import { AppPageHeader } from "./app-page-header";

describe("Authentication experience", () => {
  afterEach(() => {
    cleanup();
    navigation.push.mockReset();
    navigation.refresh.mockReset();
    navigation.pathname = "/accounts";
    vi.unstubAllGlobals();
  });

  it("renders accessible login fields", () => {
    render(<AuthForm mode="login" />);
    expect(screen.getByRole("img", { name: "Arbion" })).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: /welcome back/i }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument();
    expect(screen.getByLabelText(/password/i)).toHaveAttribute(
      "minLength",
      "12",
    );
    expect(
      screen.getByRole("link", { name: /forgot your password/i }),
    ).toHaveAttribute("href", "/forgot-password");
  });

  it("describes registration as invite-only", () => {
    render(<AuthForm mode="register" />);
    expect(
      screen.getByRole("heading", { name: /create your invited account/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/registration is limited to invited email addresses/i),
    ).toBeInTheDocument();
  });

  it("renders the branded application header with clear navigation", () => {
    render(<AppPageHeader backHref="/accounts" backLabel="Accounts" />);
    expect(screen.getByRole("img", { name: "Arbion" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Arbion home" })).toHaveAttribute(
      "href",
      "/dashboard",
    );
    expect(screen.getByRole("link", { name: /accounts/i })).toHaveAttribute(
      "href",
      "/accounts",
    );
    const appNavigation = screen.getByRole("navigation", {
      name: "Application navigation",
    });
    expect(
      within(appNavigation).getByRole("link", { name: "Dashboard" }),
    ).toHaveAttribute("href", "/dashboard");
    expect(
      within(appNavigation).getByRole("link", { name: "Portfolio" }),
    ).toHaveAttribute("aria-current", "page");
    expect(
      within(appNavigation).getByRole("link", { name: "Markets" }),
    ).toHaveAttribute("href", "/markets");
    expect(
      within(appNavigation).getByRole("link", { name: "Automations" }),
    ).toHaveAttribute("href", "/automations");
    expect(
      within(appNavigation).getByRole("link", { name: "Activity" }),
    ).toHaveAttribute("href", "/activity");
    expect(
      within(appNavigation).getByRole("link", { name: "Capital" }),
    ).toHaveAttribute("href", "/capital");
    expect(
      within(appNavigation).getByRole("link", { name: "Connections" }),
    ).toHaveAttribute("href", "/connections");
    expect(
      within(appNavigation).getByRole("link", { name: "Security" }),
    ).toHaveAttribute("href", "/settings/security");
  });

  it("retains the horizontal tab position while signed-in routes switch", () => {
    const scrollWidth = Object.getOwnPropertyDescriptor(
      HTMLElement.prototype,
      "scrollWidth",
    );
    const clientWidth = Object.getOwnPropertyDescriptor(
      HTMLElement.prototype,
      "clientWidth",
    );
    Object.defineProperty(HTMLElement.prototype, "scrollWidth", {
      configurable: true,
      get: () => 960,
    });
    Object.defineProperty(HTMLElement.prototype, "clientWidth", {
      configurable: true,
      get: () => 320,
    });
    try {
      const firstRoute = render(<AppPageHeader />);
      const firstNavigation = screen.getByRole("navigation", {
        name: "Application navigation",
      });
      firstNavigation.scrollLeft = 180;
      fireEvent.scroll(firstNavigation);
      firstRoute.unmount();

      navigation.pathname = "/markets";
      render(<AppPageHeader />);
      const nextNavigation = screen.getByRole("navigation", {
        name: "Application navigation",
      });
      expect(nextNavigation.scrollLeft).toBe(180);
      expect(
        within(nextNavigation).getByRole("link", { name: "Markets" }),
      ).toHaveAttribute("aria-current", "page");
    } finally {
      if (scrollWidth) {
        Object.defineProperty(
          HTMLElement.prototype,
          "scrollWidth",
          scrollWidth,
        );
      } else {
        Reflect.deleteProperty(HTMLElement.prototype, "scrollWidth");
      }
      if (clientWidth) {
        Object.defineProperty(
          HTMLElement.prototype,
          "clientWidth",
          clientWidth,
        );
      } else {
        Reflect.deleteProperty(HTMLElement.prototype, "clientWidth");
      }
    }
  });

  it("completes a password then authenticator sign-in without creating an early session", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          mfa_required: true,
          challenge_token: "short-lived-challenge",
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ user: { id: "user-1" } }),
      });
    vi.stubGlobal("fetch", fetchMock);
    render(<AuthForm mode="login" />);

    fireEvent.change(screen.getByLabelText("Email"), {
      target: { value: "person@example.com" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "correct horse battery staple" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Log in" }));

    expect(
      await screen.findByRole("heading", { name: /confirm it’s you/i }),
    ).toBeInTheDocument();
    expect(navigation.push).not.toHaveBeenCalled();
    fireEvent.change(screen.getByLabelText(/authenticator or recovery code/i), {
      target: { value: "123456" },
    });
    fireEvent.click(screen.getByRole("button", { name: /verify and log in/i }));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
    expect(fetchMock).toHaveBeenLastCalledWith("/api/auth/mfa/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        challenge_token: "short-lived-challenge",
        code: "123456",
      }),
    });
    expect(navigation.push).toHaveBeenCalledWith("/dashboard");
  });
});
