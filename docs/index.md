# Monaco docs

Monaco is an iOS app where friends pool USDC into a shared pot (a **cabal**), vote on which tokenized US stocks to buy on Solana, and compete on leaderboards. A cabal can also hand part of its pot to a trading agent.

## Start here

Read these in order to understand the repo:

1. **[Product](product.md)**: what the app does, the words it uses, and the rules for cabals, votes, shares and money in and out.
2. **[Architecture](architecture.md)**: the parts of the system that stay true through the backend rewrite: outside services, wallets, and how each money flow works.
3. **[Architecture decisions](architecture/README.md)**: the decision log, one file per topic, with the reasoning and what the code has not caught up with yet.
4. **[Backend platform](architecture/backend-platform.md)**: the target design for the rewritten backend. When it and `architecture.md` disagree, this file states the target and `architecture.md` states the present.
5. **[README](https://github.com/Munyon-Canyon/monaco/blob/main/README.md)**: clone, configure and run everything locally.

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
| [Agent workflow setup](agents/setup.md) | Giving a clone the plugins, model roles, skills and rules the agent workflow runs with |
| [Ship a ticket](how-to/ship-a-ticket.md) | Taking one ticket from its issue to a merge on `staging`, as a person or an agent owner |
| [Run a milestone](how-to/run-a-milestone.md) | Orchestrating a milestone: tickets, batches, owners and verifiers, landing, restacks and the promotion of `staging` into `main` |
| [Connect a trading agent](how-to/connect-an-agent.md) | Hooking up ClawPump or any LLM agent |
| [Demo checklist](how-to/demo-checklist.md) | A manual end-to-end pass before a demo |
| [Run on the local simulator](how-to/local-simulator.md) | Simulator signing and keychain issues |
| [Add a mobile feature](how-to/mobile-feature.md) | Layout, model shape and host tests for a screen on the generated client. Copy `SystemPingModel` |
| [Add a mobile route, deep link, tab or section](how-to/mobile-navigation.md) | Wiring a screen into the app shell: routes, deep links, tab roots and section slots |
| [Debug login](how-to/debug-login.md) | "I can't sign in" |
| [Overnight QA](how-to/overnight-qa.md) | The nightly test and screenshot run, and what CI runs |
| [App journeys](journeys/README.md) | The journey docs QA is built from, their XCUITests, and how to run and measure them |
| [Read iOS app logs](how-to/read-ios-logs.md) | Matching a Console.app line to an API request, crash diagnostics |
| [Gardener](how-to/gardener.md) | The nightly dead-code, candidate-lint and generator-drift report |
| [TestFlight](https://github.com/Munyon-Canyon/monaco/blob/main/apps/mobile/TestFlight.md) | Shipping an iOS build |

## Operations

| Doc | For |
| --- | --- |
| [ops-profile-photos.md](ops-profile-photos.md) | Where profile photos are stored |

## Other folders

- [`demo/`](demo/storyboard.md): the demo film's storyboard. Recording scripts are in `scripts/demo`.
- [`legacy/`](legacy/README.md): older versions of the codebase and their docs, including the docs for the deleted legacy backend (`api.md`, `ops-observability.md`, `ops-sweep-usdc.md`, the full old `architecture.md`) and the build history. Not updated. Trust the docs above over anything in it.

## Conventions for these docs

- Product rules go in `product.md`. What stays true about the system goes in `architecture.md`. Why it is built that way, and decisions the code has not caught up with, go in `architecture/`, one file per topic, with its dated log in `architecture/log/<topic>.md`. How to run things goes in the README or `how-to/`.
- Link to code by path rather than copying it.
- When code changes a rule or a flow, update the doc in the same pull request.
