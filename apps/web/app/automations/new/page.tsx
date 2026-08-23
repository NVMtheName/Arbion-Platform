import { AppPageHeader } from "../../app-page-header";
import AutomationBuilder from "./automation-builder";

export default function NewAutomation() {
  return (
    <main className="connections-page automation-page">
      <AppPageHeader backHref="/automations" backLabel="Strategies" />
      <p className="eyebrow">STRATEGY SETUP</p>
      <h1>Choose how Arbion should work.</h1>
      <p className="lede">
        Select a connected account, the AI model you trust, a strategy, and the
        amount of capital it may use. Advanced controls stay out of the way.
      </p>
      <AutomationBuilder />
    </main>
  );
}
