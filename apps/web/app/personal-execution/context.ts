import { executionResponseJSON } from "./client";
import { executionObject } from "./contract";

export type OwnerExecutionContext =
  | { available: false }
  | {
      available: true;
      product_id: string;
      account_label: string;
      session_binding: string;
    };

export function parseOwnerExecutionContext(
  value: unknown,
): OwnerExecutionContext {
  const object = executionObject(
    value,
    ["available"],
    ["product_id", "account_label", "session_binding"],
  );
  if (object.available === false) {
    executionObject(value, ["available"]);
    return { available: false };
  }
  if (
    object.available !== true ||
    typeof object.product_id !== "string" ||
    !/^[A-Z][A-Z0-9]{0,15}-USD$/.test(object.product_id) ||
    object.product_id === "USD-USD" ||
    typeof object.account_label !== "string" ||
    object.account_label.length < 1 ||
    object.account_label.length > 120 ||
    object.account_label.trim() !== object.account_label ||
    /[\u0000-\u001f\u007f]/.test(object.account_label) ||
    typeof object.session_binding !== "string" ||
    !/^[0-9a-f]{64}$/.test(object.session_binding)
  )
    throw new Error("Execution context is unavailable.");
  return {
    available: true,
    product_id: object.product_id,
    account_label: object.account_label,
    session_binding: object.session_binding,
  };
}

export async function readOwnerExecutionContext(response: Response) {
  if (response.status !== 200 || response.redirected)
    throw new Error("Execution context is unavailable.");
  return parseOwnerExecutionContext(await executionResponseJSON(response));
}
