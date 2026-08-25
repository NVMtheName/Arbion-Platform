import { AppPageHeader } from "../../app-page-header";
import AutomationBuilder from "./automation-builder";

export default function NewAutomation() {
  return (
    <main className="connections-page automation-page">
      <AppPageHeader backHref="/automations" backLabel="Automations" />
      <p className="eyebrow">ARBION AI ENGINE</p>
      <h1>Launch an autonomous shadow analyst.</h1>
      <p className="lede">
        Connect real portfolio context to a bounded AI mandate, then watch every
        proposed decision pass through Arbion&apos;s deterministic risk
        controls.
      </p>
      <AutomationBuilder />
    </main>
  );
}
