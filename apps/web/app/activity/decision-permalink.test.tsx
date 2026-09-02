import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { DecisionPermalink } from "./decision-permalink";

const originalClipboard = Object.getOwnPropertyDescriptor(
  navigator,
  "clipboard",
);

function installClipboard(writeText: (value: string) => Promise<void>) {
  Object.defineProperty(navigator, "clipboard", {
    configurable: true,
    value: { writeText },
  });
}

describe("DecisionPermalink", () => {
  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
    if (originalClipboard) {
      Object.defineProperty(navigator, "clipboard", originalClipboard);
    } else {
      Reflect.deleteProperty(navigator, "clipboard");
    }
  });

  it("copies the exact absolute record URL and confirms success", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    installClipboard(writeText);
    render(
      <DecisionPermalink href="/activity?cursor=opaque+cursor&view=paper&decision=decision-1#decision-decision-1" />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Copy exact link" }));

    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith(
        "http://localhost:3000/activity?cursor=opaque+cursor&view=paper&decision=decision-1#decision-decision-1",
      ),
    );
    expect(
      screen.getByRole("button", { name: "Link copied" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("The durable owner-scoped record link is ready."),
    ).toHaveAttribute("aria-live", "polite");
  });

  it("fails clearly without replacing the exact-link fallback", async () => {
    installClipboard(vi.fn().mockRejectedValue(new Error("blocked")));
    render(<DecisionPermalink href="/activity#decision-1" />);

    fireEvent.click(screen.getByRole("button", { name: "Copy exact link" }));

    expect(
      await screen.findByText(
        "Copy is unavailable. Open the exact record and copy its address instead.",
      ),
    ).toHaveClass("is-error");
    expect(
      screen.getByRole("link", { name: "Open exact record" }),
    ).toHaveAttribute("href", "/activity#decision-1");
  });
});
