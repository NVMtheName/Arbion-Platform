import Link from "next/link";
import AutomationBuilder from "./automation-builder";

export default function NewAutomation() {
  return (
    <main className="connections-page automation-page">
      <Link href="/automations">← Automations</Link>
      <p className="eyebrow">CONFIGURATION FOUNDATION</p>
      <h1>Create Automation</h1>
      <AutomationBuilder />
    </main>
  );
}
