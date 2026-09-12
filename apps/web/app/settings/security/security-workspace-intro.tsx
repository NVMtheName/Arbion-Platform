export function SecurityWorkspaceIntro({
  email,
  emailVerified,
}: {
  email: string;
  emailVerified: boolean;
}) {
  return (
    <>
      <section className="security-workspace-intro">
        <p className="eyebrow">ACCOUNT SECURITY</p>
        <h1 id="security-page-title">Security &amp; access</h1>
        <p className="security-note">
          Signed in as {email}. Email verification is{" "}
          {emailVerified ? "complete" : "not yet enabled for private testing"}.
        </p>
      </section>
      <nav className="security-section-links" aria-label="Security sections">
        <a href="#security-authenticator">Authenticator</a>
        <a href="#security-password">Password</a>
        <a href="#security-sessions">Browser sessions</a>
        <a href="#security-activity">Saved activity</a>
      </nav>
    </>
  );
}
