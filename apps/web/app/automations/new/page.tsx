import { AppPageHeader } from "../../app-page-header";
import AutomationBuilder from "./automation-builder";

export default function NewAutomation() {
  return (
    <main className="connections-page automation-page">
      <AppPageHeader backHref="/automations" backLabel="Automations" />
      <p className="eyebrow">CONFIGURATION FOUNDATION</p>
      <h1>Create Automation</h1>
      <AutomationBuilder />
    </main>
  );
}
