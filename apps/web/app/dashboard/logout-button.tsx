"use client";
import { useRouter } from "next/navigation";
export function LogoutButton() {
  const router = useRouter();
  return (
    <button
      className="secondary"
      onClick={async () => {
        await fetch("/api/auth/logout", { method: "POST" });
        router.replace("/login");
        router.refresh();
      }}
    >
      Log out
    </button>
  );
}
