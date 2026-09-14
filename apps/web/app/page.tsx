import { LandingExperience } from "./landing-experience";

// Reject request-time session cookies and uncached reads on the public route.
// Account data belongs exclusively to authenticated routes.
export const dynamic = "error";

export default function Home() {
  return <LandingExperience />;
}
