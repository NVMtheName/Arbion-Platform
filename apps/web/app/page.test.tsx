import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: vi.fn(), refresh: vi.fn() }),
}));

import { AuthForm } from "./auth-form";

describe("Authentication experience", () => {
  it("renders accessible login fields", () => {
    render(<AuthForm mode="login" />);
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
});
