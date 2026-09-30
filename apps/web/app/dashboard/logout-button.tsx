"use client";
import { useRouter } from "next/navigation";
import { notifyExecutionLogout } from "../personal-execution/session-events";
export function LogoutButton() {
  const router = useRouter();
  return (
    <button
      className="secondary"
      onClick={async () => {
        notifyExecutionLogout();
        await fetch("/api/auth/logout", { method: "POST" });
        router.replace("/login");
        router.refresh();
      }}
    >
      Log out
    </button>
  );
}
