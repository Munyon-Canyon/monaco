# Monaco docs

Monaco is an iOS app where friends pool USDC into a shared pot (a **cabal**), vote on which tokenized US stocks to buy on Solana, and compete on leaderboards. A cabal can also hand part of its pot to a trading agent.

## Start here

Read these in order to understand the repo:

1. **[Product](product.md)**: what the app does, the words it uses, and the rules for cabals, votes, shares and money in and out.
2. **[Architecture](architecture.md)**: the parts of the system that stay true through the backend rewrite: outside services, wallets, and how each money flow works.
3. **[Backend platform](architecture/backend-platform.md)**: the target design for the rewritten backend. When it and `architecture.md` disagree, this file states the target and `architecture.md` states the present.
4. **[README](../README.md)**: clone, configure and run everything locally.

## Reference

| Doc | Read it when |
| --- | --- |
| [architecture/backend-platform.md](architecture/backend-platform.md) | You are building the new backend: rings, lint, tests, flows, NATS, deploy. The rewrite's source of truth |
| [architecture/ci.md](architecture/ci.md) | You are changing CI: what runs on which PRs, runner choice, minute budget |
| [agent-trading.md](agent-trading.md) | You are building or debugging a trading agent |
| [design.md](design.md) | You are changing the iOS UI: colours, type, layout rules |
| [multi-user-verification.md](multi-user-verification.md) | You changed auth, membership, money or boards and need to re-verify |

## How-to

| Guide | For |
| --- | --- |
| [Connect a trading agent](how-to/connect-an-agent.md) | Hooking up ClawPump, any LLM agent, or `agents/momentum-bot` |
| [Demo checklist](how-to/demo-checklist.md) | A manual end-to-end pass before a demo |
| [Run on the local simulator](how-to/local-simulator.md) | Simulator signing and keychain issues |
| [Debug login](how-to/debug-login.md) | "I can't sign in" |
| [Overnight QA](how-to/overnight-qa.md) | The nightly test and screenshot run, and what CI runs |
| [Read iOS app logs](how-to/read-ios-logs.md) | Matching a Console.app line to an API request, crash diagnostics |
| [TestFlight](../apps/mobile/TestFlight.md) | Shipping an iOS build |

## Operations

| Doc | For |
| --- | --- |
| [ops-sweep-wallets.md](ops-sweep-wallets.md) | Emergency: moving USDC out of Privy wallets |
| [ops-profile-photos.md](ops-profile-photos.md) | Where profile photos are stored |

## Other folders

- [`demo/`](demo/storyboard.md): the demo film's storyboard. Recording scripts are in `scripts/demo`.
- [`legacy/`](legacy/README.md): older versions of the codebase and their docs, including the docs for the backend being replaced (`api.md`, `ops-observability.md`, the full old `architecture.md`) and the build history. Not updated. Trust the docs above over anything in it.

## Conventions for these docs

- Product rules go in `product.md`. What stays true about the system goes in `architecture.md`. Design decisions for the rewrite go in `architecture/`, one file per topic with a dated Log section. How to run things goes in the README or `how-to/`.
- Link to code by path rather than copying it.
- When code changes a rule or a flow, update the doc in the same pull request.
