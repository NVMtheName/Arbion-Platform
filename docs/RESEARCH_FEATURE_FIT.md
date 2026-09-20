# Quant research feature fit

Reviewed 2026-09-20 against the owner-supplied [project reference](https://docs.google.com/document/d/10BUBSX4Dsy2Jhaa-2vzfBS52JsBEptLFmoA0uuG_IGA). External projects are research references, not evidence of profitability, compatibility with Arbion accounts, or permission to execute trades. No external code, weights, dependencies, datasets, or branding are imported by this milestone.

## Implemented first: synthetic depth-sweep testing

[HFTENGINE](https://github.com/mirkovicdev/HFTENGINE) distinguishes recorded market facts from modeled fills and latency. Arbion adapts that separation of evidence and assumptions with an independently written Go **synthetic depth-sweep simulator** in the existing offline lifecycle laboratory.

It calculates hypothetical partial fills from explicit fictional bid/ask levels, applies limit prices and known-depth boundaries, and settles those results through the existing duplicate-safe exact-money journal. It does not reproduce HFTENGINE's Rust runner, recorded Binance Futures data, queue estimator, or market-making strategy. See [the laboratory contract](SIMULATION_LIFECYCLE.md#synthetic-depth-sweep).

This makes execution arithmetic and recovery testable without a scheduled model cycle. It does **not** improve the production strategy's predictive ability or establish realistic fill probabilities. Existing stored AI market evidence has aggregate depth, not individual L2 levels; no historical order book or fill is inferred from it. Production Paper/Shadow behavior remains unchanged.

## Candidate: bounded opposing analyst views

[TradingAgents](https://github.com/TauricResearch/TradingAgents) offers specialist analysts, opposing research arguments, and structured synthesis. Its current [license](https://github.com/TauricResearch/TradingAgents/blob/main/LICENSE) is Apache-2.0. Its graph and data dependencies are not a broker adapter or Arbion's deterministic control plane. The [paper](https://arxiv.org/html/2412.20138v7) reports 11 model calls and more than 20 tool calls per prediction for its experiment; this is not a cost or latency guarantee for current code.

A future bounded experiment could consume one immutable, point-in-time Arbion evidence bundle, produce evidence-linked bull/bear summaries and a typed abstain/proposal synthesis, and compare it with the existing baseline. Keep hard call/token/deadline budgets, missing-evidence flags, and exact route attribution. Start with fixtures, then separately authorize scheduled Shadow evaluation. Go retains all sizing, authorization, risk, veto, and execution eligibility. No new agent graph, provider call, tool access, credentials, or runtime flag is added here.

## Candidate: sentiment features, not an RL replacement

The reference pairs [HARLF](https://arxiv.org/html/2507.18560v1) with [franjgs/llm-rl-finance-trader](https://github.com/franjgs/llm-rl-finance-trader), but they are different projects. The latter credits another paper and implements single-stock PPO experiments; HARLF describes hierarchical portfolio allocation and excludes transaction costs from its experiment. Neither establishes production suitability.

[FinBERT](https://github.com/ProsusAI/finBERT) can classify financial-text sentiment. Source licensing does not by itself resolve model-weight or news-data rights. A future study requires approved pinned weights, licensed timestamped inputs, duplicate and relevance handling, an explicit long-text policy, and an offline comparison with/without sentiment. Use chronological holdouts, costs and simple baselines before considering RL. Sentiment is evidence, not a return forecast, risk approval, or trading command.

## Candidate: constrained strategy discovery

The [alpha-discovery paper](https://arxiv.org/abs/2409.06289) suggests proposing and evaluating formulaic factors. The linked [repository](https://github.com/kouzhizhuo/Automate-Strategy-Finding-with-LLM-in-Quant-investment) has no license file in the inspected tree; its main script depends on Ricequant and evaluates formulas with Python `eval`. Do not copy that code or evaluate untrusted model formulas.

A separately scoped future implementation should use an independently written allowlisted formula grammar, bounded deterministic evaluation, point-in-time provenance, chronological walk-forward validation, transaction costs, multiple-testing controls, and an untouched final holdout. Candidates cannot self-promote into a mandate or bypass risk. This milestone does not add formula execution, training, data subscriptions, or production strategy changes.
