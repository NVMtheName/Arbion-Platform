"use client";

import { useRef, useState } from "react";

const views = ["Portfolio", "AI Engine", "Decision Journal"] as const;

/** Marketing illustration only. This component never reads an account or calls an API. */
export function ProductPreview() {
  const [selected, setSelected] = useState(0);
  const tabs = useRef<(HTMLButtonElement | null)[]>([]);

  return (
    <section
      className="product-preview"
      aria-label="Illustrative Arbion command-center preview"
    >
      <header className="product-preview-chrome">
        <span>
          <i aria-hidden="true" /> ARBION / WORKSPACE
        </span>
        <span className="product-preview-label">Illustrative preview</span>
      </header>
      <div
        className="product-preview-tabs"
        role="tablist"
        aria-label="Explore the workspace"
      >
        {views.map((view, index) => (
          <button
            key={view}
            ref={(element) => {
              tabs.current[index] = element;
            }}
            type="button"
            role="tab"
            id={`preview-tab-${index}`}
            aria-selected={selected === index}
            aria-controls={`preview-panel-${index}`}
            tabIndex={selected === index ? 0 : -1}
            onClick={() => setSelected(index)}
            onKeyDown={(event) => {
              const next =
                event.key === "ArrowRight"
                  ? (index + 1) % views.length
                  : event.key === "ArrowLeft"
                    ? (index + views.length - 1) % views.length
                    : event.key === "Home"
                      ? 0
                      : event.key === "End"
                        ? views.length - 1
                        : undefined;
              if (next === undefined) return;
              event.preventDefault();
              setSelected(next);
              tabs.current[next]?.focus();
            }}
          >
            {view}
          </button>
        ))}
      </div>
      <div className="product-preview-stage">
        <div
          role="tabpanel"
          id="preview-panel-0"
          aria-labelledby="preview-tab-0"
          hidden={selected !== 0}
          tabIndex={0}
        >
          <div className="product-preview-value">
            <span>Connected portfolio</span>
            <strong>
              $24,680<span>.50</span>
            </strong>
            <small>Example values · not account data</small>
          </div>
          <div className="product-preview-allocation" aria-hidden="true">
            <span />
            <span />
          </div>
          <div className="product-preview-account">
            <span className="preview-provider">S</span>
            <div>
              <strong>Brokerage</strong>
              <small>Charles Schwab</small>
            </div>
            <b>$18,240.00</b>
          </div>
          <div className="product-preview-account">
            <span className="preview-provider is-crypto">C</span>
            <div>
              <strong>Digital assets</strong>
              <small>Coinbase</small>
            </div>
            <b>$6,440.50</b>
          </div>
          <p className="product-preview-note">
            One view. Assets stay with your provider.
          </p>
        </div>
        <div
          role="tabpanel"
          id="preview-panel-1"
          aria-labelledby="preview-tab-1"
          hidden={selected !== 1}
          tabIndex={0}
        >
          <div className="product-preview-section-title">
            <span>AI Engine</span>
            <b>Paper simulation</b>
          </div>
          <h3>Intelligence, with a boundary.</h3>
          <p>
            Your account context and selected model. A deterministic risk check
            before every simulated action.
          </p>
          <ol className="product-preview-flow">
            {[
              ["01", "Account context", "Provider-stamped inputs"],
              ["02", "Your AI model", "A proposal, not permission"],
              ["03", "Risk controls", "Capital and mandate limits"],
              ["04", "Saved evidence", "Review the full decision"],
            ].map(([number, title, detail]) => (
              <li key={number}>
                <span>{number}</span>
                <div>
                  <strong>{title}</strong>
                  <small>{detail}</small>
                </div>
              </li>
            ))}
          </ol>
        </div>
        <div
          role="tabpanel"
          id="preview-panel-2"
          aria-labelledby="preview-tab-2"
          hidden={selected !== 2}
          tabIndex={0}
        >
          <div className="product-preview-section-title">
            <span>Decision Journal</span>
            <b>Example record</b>
          </div>
          <h3>Understand the decision.</h3>
          <div className="product-preview-decision">
            <span>ABSTAIN</span>
            <strong>No action proposed</strong>
            <p>
              When evidence is insufficient, the engine can hold. The reasoning
              stays available for review.
            </p>
          </div>
          <dl className="product-preview-evidence">
            <div>
              <dt>Inputs</dt>
              <dd>Source & observation time</dd>
            </div>
            <div>
              <dt>AI route</dt>
              <dd>Provider, model & profile</dd>
            </div>
            <div>
              <dt>Result</dt>
              <dd>Immutable non-live evidence</dd>
            </div>
          </dl>
        </div>
      </div>
      <footer className="product-preview-footer">
        <span>
          <i aria-hidden="true" /> No live execution path
        </span>
        <span>No live prices shown</span>
      </footer>
    </section>
  );
}
