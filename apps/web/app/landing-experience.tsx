"use client";

import { motion } from "motion/react";
import Link from "next/link";

import { ArbionBrand } from "./brand";

const reveal = {
  hidden: { opacity: 0, y: 20 },
  visible: { opacity: 1, y: 0 },
};

const previewCells = [
  0, 1, 2, 1, 0, 2, 3, 1, 3, 2, 0, 1, 2, 4, 2, 1, 3, 4, 2, 1, 0, 2, 1, 3, 4, 2,
  3, 1, 2, 4, 3, 2, 4, 3, 1,
];

const capabilities = [
  {
    index: "01",
    title: "Every signal keeps its source.",
    copy: "Feed, venue, quality, and freshness stay attached to every observation—so a single-venue quote can never masquerade as a consolidated market view.",
  },
  {
    index: "02",
    title: "Risk is a control plane.",
    copy: "Capital limits, protected reserves, mandates, and circuit breakers remain deterministic. AI can explain a decision; it cannot bypass one.",
  },
  {
    index: "03",
    title: "Your evidence is durable.",
    copy: "PAPER simulations and SHADOW intent become a reviewable journal, so every proposal has context instead of disappearing into a chat transcript.",
  },
];

export function LandingExperience() {
  return (
    <main className="landing-page">
      <nav className="landing-nav" aria-label="Primary navigation">
        <ArbionBrand className="landing-brand" href="/" priority />
        <div className="landing-nav-links">
          <a href="#platform">Platform</a>
          <a href="#trust">Trust</a>
          <Link href="/login">Sign in</Link>
          <Link className="landing-nav-cta" href="/register">
            Create invited account
          </Link>
        </div>
      </nav>

      <section className="landing-hero">
        <motion.div
          className="landing-hero-copy"
          initial="hidden"
          animate="visible"
          transition={{ staggerChildren: 0.1 }}
        >
          <motion.p className="landing-kicker" variants={reveal}>
            FINANCIAL INTELLIGENCE · HUMAN CONTROL
          </motion.p>
          <motion.h1 variants={reveal}>
            See your money
            <span>as a system.</span>
          </motion.h1>
          <motion.p className="landing-lede" variants={reveal}>
            Arbion brings market intelligence, portfolio context, automation,
            and decision evidence into one source-aware command center—without
            handing control to a black box.
          </motion.p>
          <motion.div className="landing-hero-actions" variants={reveal}>
            <Link className="landing-primary-action" href="/login">
              Enter command center <span aria-hidden="true">↗</span>
            </Link>
            <a className="landing-secondary-action" href="#platform">
              Explore the platform
            </a>
          </motion.div>
          <motion.ul className="landing-proof-list" variants={reveal}>
            <li>Read-only broker access</li>
            <li>Source-stamped market data</li>
            <li>No live execution path</li>
          </motion.ul>
        </motion.div>

        <motion.div
          className="landing-terminal"
          initial={{ opacity: 0, scale: 0.96, rotateY: 4 }}
          animate={{ opacity: 1, scale: 1, rotateY: 0 }}
          transition={{ delay: 0.2, duration: 0.75 }}
          aria-label="Illustrative Arbion command-center preview"
        >
          <header>
            <div>
              <span className="terminal-overline">COMMAND SURFACE</span>
              <strong>Market awareness</strong>
            </div>
            <span className="terminal-status">
              <i /> SOURCE AWARE
            </span>
          </header>
          <div className="terminal-chart" aria-hidden="true">
            <svg viewBox="0 0 640 250" role="presentation">
              <defs>
                <linearGradient id="arbion-area" x1="0" x2="0" y1="0" y2="1">
                  <stop offset="0%" stopColor="#35e7a0" stopOpacity="0.34" />
                  <stop offset="100%" stopColor="#35e7a0" stopOpacity="0" />
                </linearGradient>
              </defs>
              <g className="terminal-grid-lines">
                <line x1="0" x2="640" y1="50" y2="50" />
                <line x1="0" x2="640" y1="125" y2="125" />
                <line x1="0" x2="640" y1="200" y2="200" />
              </g>
              <motion.path
                className="terminal-area"
                d="M0 210 C55 202 78 164 132 178 S224 112 280 132 S368 70 420 94 S506 44 560 62 S610 24 640 34 L640 250 L0 250 Z"
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                transition={{ delay: 0.55, duration: 0.8 }}
              />
              <motion.path
                className="terminal-line"
                d="M0 210 C55 202 78 164 132 178 S224 112 280 132 S368 70 420 94 S506 44 560 62 S610 24 640 34"
                fill="none"
                initial={{ pathLength: 0 }}
                animate={{ pathLength: 1 }}
                transition={{ delay: 0.45, duration: 1.4 }}
              />
            </svg>
            <div className="terminal-chart-label">
              <span>Illustrative interface preview</span>
              <strong>No live prices shown</strong>
            </div>
          </div>
          <div className="terminal-lower-grid">
            <section>
              <span className="terminal-overline">SIGNAL COVERAGE</span>
              <div className="terminal-heatmap" aria-hidden="true">
                {previewCells.map((level, index) => (
                  <motion.i
                    className={`level-${level}`}
                    initial={{ opacity: 0, scale: 0.6 }}
                    animate={{ opacity: 1, scale: 1 }}
                    transition={{ delay: 0.65 + index * 0.012 }}
                    key={`${level}-${index}`}
                  />
                ))}
              </div>
            </section>
            <section className="terminal-controls">
              <span className="terminal-overline">CONTROL PLANE</span>
              <p>
                <i className="is-ready" /> Risk policy active
              </p>
              <p>
                <i /> Execution unavailable
              </p>
              <p>
                <i className="is-info" /> Evidence recorded
              </p>
            </section>
          </div>
        </motion.div>
      </section>

      <section className="landing-marquee" aria-label="Platform coverage">
        <div>
          <span>PORTFOLIO CONTEXT</span>
          <i />
          <span>MARKET INTELLIGENCE</span>
          <i />
          <span>RISK CONTROLS</span>
          <i />
          <span>DECISION EVIDENCE</span>
          <i />
          <span>NON-LIVE AUTOMATION</span>
        </div>
      </section>

      <section className="landing-section" id="platform">
        <motion.div
          className="landing-section-heading"
          initial={{ opacity: 0, y: 20 }}
          whileInView={{ opacity: 1, y: 0 }}
          viewport={{ once: true, amount: 0.35 }}
        >
          <p className="landing-kicker">CLARITY BEFORE ACTION</p>
          <h2>A command center built around truth.</h2>
          <p>
            The most useful interface is not the one with the most flashing
            numbers. It is the one that makes context, confidence, and control
            impossible to miss.
          </p>
        </motion.div>
        <div className="landing-capability-grid">
          {capabilities.map((capability, index) => (
            <motion.article
              key={capability.index}
              initial={{ opacity: 0, y: 24 }}
              whileInView={{ opacity: 1, y: 0 }}
              whileHover={{ y: -6 }}
              viewport={{ once: true, amount: 0.3 }}
              transition={{ delay: index * 0.08 }}
            >
              <span>{capability.index}</span>
              <h3>{capability.title}</h3>
              <p>{capability.copy}</p>
            </motion.article>
          ))}
        </div>
      </section>

      <section className="landing-trust" id="trust">
        <div>
          <p className="landing-kicker">DESIGNED TO EARN TRUST</p>
          <h2>Intelligence with boundaries.</h2>
        </div>
        <div className="landing-trust-grid">
          <article>
            <span>01</span>
            <strong>Broker truth stays separate</strong>
            <p>
              Connected accounts remain authoritative for balances and
              positions.
            </p>
          </article>
          <article>
            <span>02</span>
            <strong>AI stays advisory</strong>
            <p>
              Models can interpret context, never authorize or execute a trade.
            </p>
          </article>
          <article>
            <span>03</span>
            <strong>Execution stays unavailable</strong>
            <p>
              PAPER and SHADOW modes create evidence without sending broker
              orders.
            </p>
          </article>
        </div>
      </section>

      <section className="landing-final-cta">
        <p className="landing-kicker">ARBION PRIVATE PREVIEW</p>
        <h2>Turn financial noise into a disciplined operating system.</h2>
        <div className="landing-hero-actions">
          <Link className="landing-primary-action" href="/login">
            Sign in to Arbion <span aria-hidden="true">↗</span>
          </Link>
          <Link className="landing-secondary-action" href="/register">
            Create invited account
          </Link>
        </div>
      </section>

      <footer className="landing-footer">
        <ArbionBrand className="landing-footer-brand" href="/" />
        <p>Source-aware intelligence. Human-controlled decisions.</p>
        <span>© {new Date().getUTCFullYear()} Arbion</span>
      </footer>
    </main>
  );
}
