export function AccountDetailNavigation({
  portfolio,
}: {
  portfolio: "crypto" | "broker" | "unavailable";
}) {
  return (
    <nav className="account-section-links" aria-label="Account sections">
      <a href="#account-overview">Overview</a>
      {portfolio !== "unavailable" && (
        <a
          href={
            portfolio === "crypto"
              ? "#crypto-position-title"
              : "#holdings-command-title"
          }
        >
          Holdings
        </a>
      )}
      <a href="#dashboard-input-chain-title">AI input evidence</a>
      <a href="#account-sync-history-title">Saved syncs</a>
      {portfolio !== "unavailable" && (
        <a href="#reconciliation-title">Reconciliation</a>
      )}
    </nav>
  );
}
