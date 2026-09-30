// This ephemeral notification only closes workspaces; it conveys no authority,
// identity, credential, evidence, or order data and is never persisted.
export const executionLogoutEvent = "arbion-execution-logout";
export const executionSessionChannel = "arbion-execution-session";

export function notifyExecutionLogout() {
  window.dispatchEvent(new Event(executionLogoutEvent));
  if (typeof BroadcastChannel === "undefined") return;
  let channel: BroadcastChannel | undefined;
  try {
    channel = new BroadcastChannel(executionSessionChannel);
    channel.postMessage("logout");
  } catch {
    // Browser privacy restrictions must not prevent the actual server logout.
    // A remote tab still has the server's current session binding as its guard.
  } finally {
    try {
      channel?.close();
    } catch {
      // Cleanup must not interfere with the server logout either.
    }
  }
}
