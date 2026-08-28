import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ConnectionsManager } from "./connections-manager";

describe("ConnectionsManager", () => {
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("makes provider connection and model choice the primary flow", () => {
    render(
      <ConnectionsManager
        entitled
        initialConnections={[]}
        initialPreference={null}
        providers={[
          {
            id: "openai",
            label: "OpenAI",
            credential_types: ["api_key"],
            capabilities: ["text"],
          },
        ]}
      />,
    );

    expect(
      screen.getByRole("heading", { name: "Connect an AI provider" }),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Add API key" }));
    expect(screen.getByLabelText(/Connection name/)).not.toBeRequired();
    expect(screen.getByLabelText("API key")).toHaveAttribute(
      "type",
      "password",
    );
    expect(
      screen.getByRole("button", { name: "Save API key" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Choose your default model" }),
    ).toBeInTheDocument();
  });

  it("explains staged credential rotation before accepting a new key", () => {
    render(
      <ConnectionsManager
        entitled
        initialConnections={[
          {
            id: "connection-1",
            provider: "openai",
            provider_label: "OpenAI",
            display_name: "My OpenAI",
            status: "active",
            enabled: true,
            credential_hint: "••••1234",
          },
        ]}
        initialPreference={null}
        providers={[
          {
            id: "openai",
            label: "OpenAI",
            credential_types: ["api_key"],
            capabilities: ["text"],
          },
        ]}
      />,
    );

    fireEvent.click(screen.getByText("Manage connection"));
    fireEvent.click(screen.getByRole("button", { name: "Replace key" }));
    expect(screen.getByText(/current key stays active/i)).toBeVisible();
    expect(
      screen.getByRole("button", {
        name: "Verify and rotate key",
      }),
    ).toBeInTheDocument();
  });

  it("loads and preserves the current verified model preference", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        models: [
          { id: "gpt-5.6", display_name: "GPT-5.6", provider: "openai" },
        ],
      }),
    });
    vi.stubGlobal("fetch", fetchMock);

    render(
      <ConnectionsManager
        entitled
        initialConnections={[
          {
            id: "connection-1",
            provider: "openai",
            provider_label: "OpenAI",
            display_name: "My OpenAI",
            status: "active",
            enabled: true,
            credential_hint: "••••1234",
          },
        ]}
        initialPreference={{
          connection_id: "connection-1",
          model_id: "gpt-5.6",
        }}
        providers={[
          {
            id: "openai",
            label: "OpenAI",
            credential_types: ["api_key"],
            capabilities: ["text"],
          },
        ]}
      />,
    );

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/connections/ai/connection-1/models",
        expect.objectContaining({ method: "GET" }),
      ),
    );
    expect(screen.getByLabelText("Connection")).toHaveValue("connection-1");
    expect(screen.getByLabelText("Model")).toHaveValue("gpt-5.6");
    expect(screen.getByText("Current default: gpt-5.6")).toBeVisible();
  });
});
