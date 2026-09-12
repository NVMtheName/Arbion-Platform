import { AppPageHeader } from "../../app-page-header";
import AutomationBuilder from "./automation-builder";
import { AutomationBuilderIntroduction } from "./automation-builder-introduction";

export default function NewAutomation() {
  return (
    <main className="connections-page automation-page automation-builder-page command-content-continuity">
      <AppPageHeader
        backHref="/automations"
        backLabel="Automations"
        contentHeadingId="automation-builder-title"
      />
      <AutomationBuilderIntroduction />
      <AutomationBuilder />
    </main>
  );
}
