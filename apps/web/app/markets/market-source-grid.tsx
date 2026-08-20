export type MarketSource = {
  id: string;
  label: string;
  role: string;
  feed: string;
  quality: string;
  capabilities: string[];
  enabled: boolean;
  healthy: boolean;
};

function readable(value: string) {
  return value.replaceAll("_", " ").toLowerCase();
}

function sourceStatus(source: MarketSource) {
  if (!source.enabled) return "Not configured";
  return source.healthy ? "Available" : "Degraded";
}

export function MarketSourceGrid({ sources }: { sources: MarketSource[] }) {
  return (
    <section className="market-source-grid" aria-label="Market data sources">
      {sources.map((source) => (
        <article className="market-source-card" key={source.id}>
          <header>
            <div>
              <p className="market-source-role">{readable(source.role)}</p>
              <h3>{source.label}</h3>
            </div>
            <span
              className={`market-source-status ${source.enabled && source.healthy ? "available" : source.enabled ? "degraded" : "disabled"}`}
            >
              {sourceStatus(source)}
            </span>
          </header>
          <dl>
            <div>
              <dt>Feed</dt>
              <dd>{source.feed}</dd>
            </div>
            <div>
              <dt>Quality</dt>
              <dd>{readable(source.quality)}</dd>
            </div>
          </dl>
          <ul aria-label={`${source.label} capabilities`}>
            {source.capabilities.map((capability) => (
              <li key={capability}>{readable(capability)}</li>
            ))}
          </ul>
        </article>
      ))}
    </section>
  );
}
