import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { CommissioningWorkspace } from "./commissioning";
import {
  CommissioningClientError,
  type CommissioningClient,
} from "./commissioning-client";
import { review, receipt, consent } from "./commissioning.test-fixtures";
afterEach(cleanup);
function fixture() {
  return {
    prepare: vi.fn<CommissioningClient["prepare"]>().mockResolvedValue(receipt),
    readReceipt: vi
      .fn<CommissioningClient["readReceipt"]>()
      .mockResolvedValue(receipt),
    readConsent: vi
      .fn<CommissioningClient["readConsent"]>()
      .mockRejectedValue(new CommissioningClientError("EXECUTION_NOT_FOUND")),
    approve: vi.fn<CommissioningClient["approve"]>().mockResolvedValue(consent),
    revoke: vi
      .fn<CommissioningClient["revoke"]>()
      .mockResolvedValue({ ...consent, revoked_at: "2099-10-07T12:02:00Z" }),
  };
}
function save() {
  fireEvent.click(
    screen.getByRole("button", { name: "Save exact pilot setup" }),
  );
  return screen.findByRole("heading", { name: "Exact setup saved" });
}
function approve() {
  fireEvent.change(screen.getByLabelText("Fresh setup MFA code"), {
    target: { value: "123456" },
  });
  fireEvent.click(
    screen.getByRole("button", { name: "Record consent to exact terms" }),
  );
}
describe("explicit pilot commissioning workflow", () => {
  it("shows exact reviewed terms without automatic calls and saves before fresh consent", async () => {
    const client = fixture();
    render(<CommissioningWorkspace review={review} client={client} />);
    expect(client.prepare).not.toHaveBeenCalled();
    expect(client.readReceipt).not.toHaveBeenCalled();
    expect(screen.getAllByText("100")).toHaveLength(2);
    expect(screen.getByText(review.model_id)).toBeInTheDocument();
    expect(
      screen.queryByLabelText("Fresh setup MFA code"),
    ).not.toBeInTheDocument();
    await save();
    approve();
    await screen.findByRole("heading", { name: "Historical consent receipt" });
    expect(client.approve).toHaveBeenCalledExactlyOnceWith("123456");
    expect(
      screen.queryByLabelText("Fresh setup MFA code"),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText(
        /not current authorization or proof that live trading is running/,
      ),
    ).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "Revoke recorded consent" }),
    );
    await screen.findByRole("heading", { name: "Consent revoked" });
    expect(client.revoke).toHaveBeenCalledOnce();
    expect(
      screen.getByRole("button", { name: "Revoke recorded consent" }),
    ).toBeDisabled();
  });
  it("locks synchronously against duplicate save events", async () => {
    const client = fixture();
    let resolve!: (value: typeof receipt) => void;
    client.prepare.mockImplementation(
      () =>
        new Promise((done) => {
          resolve = done;
        }),
    );
    render(<CommissioningWorkspace review={review} client={client} />);
    const button = screen.getByRole("button", {
      name: "Save exact pilot setup",
    });
    act(() => {
      button.click();
      button.click();
    });
    expect(client.prepare).toHaveBeenCalledOnce();
    await act(async () => resolve(receipt));
  });
  it("lost setup response allows only reads, and404 cannot clear uncertainty", async () => {
    const client = fixture();
    client.prepare.mockRejectedValue(new Error("private"));
    client.readReceipt.mockRejectedValueOnce(
      new CommissioningClientError("EXECUTION_NOT_FOUND"),
    );
    render(<CommissioningWorkspace review={review} client={client} />);
    fireEvent.click(
      screen.getByRole("button", { name: "Save exact pilot setup" }),
    );
    await screen.findByRole("alert");
    fireEvent.click(
      screen.getByRole("button", { name: "Read saved setup and consent" }),
    );
    await waitFor(() => expect(client.readReceipt).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Read saved setup and consent" }),
      ).toBeEnabled(),
    );
    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Save exact pilot setup" }),
    ).toBeDisabled();
    expect(screen.queryByText("private")).not.toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "Read saved setup and consent" }),
    );
    await screen.findByRole("heading", { name: "Exact setup saved" });
    await waitFor(() =>
      expect(screen.queryByRole("alert")).not.toBeInTheDocument(),
    );
    expect(client.prepare).toHaveBeenCalledOnce();
  });
  it("lost consent response clears MFA and recovers without another approval, including a404 first", async () => {
    const client = fixture();
    client.approve.mockRejectedValue(new Error("private"));
    render(<CommissioningWorkspace review={review} client={client} />);
    await save();
    approve();
    await screen.findByRole("alert");
    expect(screen.getByLabelText("Fresh setup MFA code")).toHaveValue("");
    expect(
      screen.getByRole("button", { name: "Record consent to exact terms" }),
    ).toBeDisabled();
    fireEvent.click(
      screen.getByRole("button", { name: "Read saved setup and consent" }),
    );
    await waitFor(() => expect(client.readConsent).toHaveBeenCalledOnce());
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Read saved setup and consent" }),
      ).toBeEnabled(),
    );
    expect(screen.getByRole("alert")).toBeInTheDocument();
    client.readConsent.mockResolvedValue(consent);
    fireEvent.click(
      screen.getByRole("button", { name: "Read saved setup and consent" }),
    );
    await screen.findByRole("heading", { name: "Historical consent receipt" });
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(client.approve).toHaveBeenCalledOnce();
  });
  it("revocation uncertainty remains until a read actually shows revocation", async () => {
    const client = fixture();
    client.readConsent.mockResolvedValue(consent);
    client.revoke.mockRejectedValue(new Error("private"));
    render(<CommissioningWorkspace review={review} client={client} />);
    fireEvent.click(
      screen.getByRole("button", { name: "Read saved setup and consent" }),
    );
    await screen.findByRole("heading", { name: "Historical consent receipt" });
    fireEvent.click(
      screen.getByRole("button", { name: "Revoke recorded consent" }),
    );
    await screen.findByRole("alert");
    fireEvent.click(
      screen.getByRole("button", { name: "Read saved setup and consent" }),
    );
    await waitFor(() => expect(client.readConsent).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Read saved setup and consent" }),
      ).toBeEnabled(),
    );
    expect(screen.getByRole("alert")).toBeInTheDocument();
    client.readConsent.mockResolvedValue({
      ...consent,
      revoked_at: "2099-10-07T12:02:00Z",
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Read saved setup and consent" }),
    );
    await screen.findByRole("heading", { name: "Consent revoked" });
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(client.revoke).toHaveBeenCalledOnce();
  });
  it("expired terms permit only historical reads", () => {
    const client = fixture();
    render(
      <CommissioningWorkspace
        review={{
          ...review,
          effective_from: "2020-10-07T12:00:00Z",
          expires_at: "2020-10-08T12:00:00Z",
        }}
        client={client}
      />,
    );
    expect(
      screen.getByRole("button", { name: "Save exact pilot setup" }),
    ).toBeDisabled();
    expect(
      screen.getByRole("button", { name: "Read saved setup and consent" }),
    ).toBeEnabled();
  });
});
