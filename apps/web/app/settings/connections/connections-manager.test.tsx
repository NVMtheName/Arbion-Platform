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
            runtime_protected: false,
            removal_protected: false,
            protected_mandate_count: 0,
            active_strategy_count: 0,
            retained_automation_count: 0,
            default_model_selected: false,
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

  it("preserves known continuity facts after a verified key rotation", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        connection: {
          id: "connection-1",
          provider: "openai",
          provider_label: "OpenAI",
          display_name: "My OpenAI",
          status: "active",
          enabled: true,
          credential_hint: "••••5678",
          runtime_protected: false,
          removal_protected: false,
          protected_mandate_count: 0,
          active_strategy_count: 0,
          retained_automation_count: 0,
          default_model_selected: false,
        },
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
            runtime_protected: true,
            removal_protected: true,
            protected_mandate_count: 1,
            active_strategy_count: 1,
            retained_automation_count: 1,
            default_model_selected: true,
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
    fireEvent.change(screen.getByLabelText("New API key"), {
      target: { value: "replacement-key-material" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Verify and rotate key" }),
    );

    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        "/api/connections/ai/connection-1/credential",
        expect.objectContaining({ method: "PUT" }),
      ),
    );
    const continuity = screen.getByRole("region", {
      name: "My OpenAI connection continuity",
    });
    expect(continuity).toHaveTextContent("AI engine continuity protected");
    expect(screen.getByRole("button", { name: "Disable" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Remove" })).toBeDisabled();
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
            runtime_protected: false,
            removal_protected: true,
            protected_mandate_count: 0,
            active_strategy_count: 0,
            retained_automation_count: 0,
            default_model_selected: true,
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

  it("shows exact runtime dependencies before connection management", () => {
    render(
      <ConnectionsManager
        entitled
        initialConnections={[
          {
            id: "connection-1",
            provider: "openai",
            provider_label: "OpenAI",
            display_name: "Production OpenAI",
            status: "active",
            enabled: true,
            credential_hint: "••••1234",
            runtime_protected: true,
            removal_protected: true,
            protected_mandate_count: 1,
            active_strategy_count: 2,
            retained_automation_count: 2,
            default_model_selected: true,
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

    const continuity = screen.getByRole("region", {
      name: "Production OpenAI connection continuity",
    });
    expect(continuity).toHaveTextContent("AI engine continuity protected");
    expect(continuity).toHaveTextContent("2 active or paused engines");
    expect(continuity).toHaveTextContent("1 ready or paused mandate");
    expect(continuity).toHaveTextContent(
      "Verified key rotation preserves the same connection identity",
    );

    fireEvent.click(screen.getByText("Manage connection"));
    expect(screen.getByRole("button", { name: "Replace key" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Disable" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Remove" })).toBeDisabled();
  });

  it("keeps removal protected for retained identity without blocking disable", () => {
    render(
      <ConnectionsManager
        entitled
        initialConnections={[
          {
            id: "connection-1",
            provider: "openai",
            provider_label: "OpenAI",
            display_name: "Default OpenAI",
            status: "active",
            enabled: true,
            credential_hint: "••••1234",
            runtime_protected: false,
            removal_protected: true,
            protected_mandate_count: 0,
            active_strategy_count: 0,
            retained_automation_count: 1,
            default_model_selected: true,
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

    const continuity = screen.getByRole("region", {
      name: "Default OpenAI connection continuity",
    });
    expect(continuity).toHaveTextContent("Configuration identity retained");
    expect(continuity).toHaveTextContent("Default model");
    expect(continuity).toHaveTextContent("1 saved automation");
    fireEvent.click(screen.getByText("Manage connection"));
    expect(screen.getByRole("button", { name: "Disable" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Remove" })).toBeDisabled();
  });
});
