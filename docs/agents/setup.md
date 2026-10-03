# Agent workflow setup

This page sets up a clone so that Claude Code runs the same agent workflow the M7 milestone ran: the same plugins, model roles, skills and rules. The rules the workflow runs by are in [Standing orders](standing-orders.md). After setup, follow [Ship a ticket](../how-to/ship-a-ticket.md) to own one ticket, or [Run a milestone](../how-to/run-a-milestone.md) to orchestrate many. A cloud session follows [Cloud root](../how-to/run-a-milestone.md#cloud-root).

## Set up a clone

1. Run `just install`. Among the other tools, it runs `scripts/setup-agent-env.sh`, which writes the pstack model roles (see [Model roles](#model-roles)). It asks before it writes into `~/.claude`.
2. Open the repo in Claude Code and accept the workspace trust dialog. `.claude/settings.json` then registers the team marketplaces and enables the team plugins. Claude Code reads marketplace entries only in a trusted folder.
3. Install `superpowers` once. It is the only team plugin that its marketplace fetches from another repository, so trust alone does not install it:

        claude plugin install superpowers@claude-plugins-official --scope project

4. Run `/plugin` in Claude Code and confirm that each plugin in the table below is listed and enabled.
5. Optional: to fit the agent workflow to this machine, set `lanes`, `[check] slots` or `[dispatch] max_load` in `.git/.monaco/agents.local.toml`. The file overrides `.monaco/agents.toml` for every worktree of this clone, is never committed and accepts no other key. `monacoctl agents dispatch` prints a `local config:` line while it is in effect.

## Secrets in worktrees

`.env.local` is committed encrypted. `scripts/with-dotenv-local.sh`, which `just run backend`, `just test backend` and the other secret recipes use, finds the dotenvx private key in this order:

1. `.env.keys` in the current checkout.
2. `.env.keys` in the primary clone, the checkout that holds `.git`. The script passes its path to dotenvx with `-fk`.
3. `DOTENV_PRIVATE_KEY_LOCAL` or `DOTENV_PRIVATE_KEY` in the environment.
4. Dotenvx Armor (`dotenvx armor`).

A keys file counts only when it has a `DOTENV_PRIVATE_KEY_LOCAL=` line. The script prints the source it used to stderr and never reads or prints the key. A worktree under `.worktrees/` needs nothing extra. Never copy or symlink `.env.keys` into a worktree.

## Plugins

`.claude/settings.json` declares these plugins under `enabledPlugins` and their marketplaces under `extraKnownMarketplaces`. `claude-plugins-official` needs no marketplace entry, because Claude Code knows it by default. The M7 flow ran on the versions listed.

| Plugin | Marketplace repository | Status | Why the flow uses it | Version |
| --- | --- | --- | --- | --- |
| `pstack@pstack-claude` | `michael-denyer/pstack-claude` | Required | Owners and verifiers are `pstack:poteto-agent` spawns. `scripts/agent-guard-dispatch.py` refuses any other agent type for a spawn that carries a `brief:` line. | 0.9.43 |
| `superpowers@claude-plugins-official` | `anthropics/claude-plugins-official` | Recommended | Planning, debugging and verification skills for day-to-day work. | 6.4.1 |
| `compound-engineering@compound-engineering-plugin` | `EveryInc/compound-engineering-plugin` | Recommended | Review personas and the brainstorm, plan and work skills. | 3.6.1 |
| `swift-lsp@claude-plugins-official` | `anthropics/claude-plugins-official` | Recommended | Swift language server for `apps/mobile` and `packages/mobile-core`. | 1.0.0 |
| `swiftui-pro@swiftui-agent-skill` | `twostraws/SwiftUI-Agent-Skill` | Recommended | SwiftUI review. | 1.0.0 |
| `swiftui-expert@swiftui-expert-skill` | `AvdLee/SwiftUI-Agent-Skill` | Recommended | SwiftUI state, performance and Instruments guidance. | 3.3.0 |

The project settings hold no personal settings: no theme, status line, model or effort. Keep personal plugins and settings in `~/.claude/settings.json`.

To turn off a recommended plugin for yourself, set it to `false` under `enabledPlugins` in `.claude/settings.local.json`, your personal project settings, and never commit that file. Never turn off `pstack`.

## Model roles

pstack reads its per-role models from `~/.claude/pstack-models.md`, which `~/.claude/CLAUDE.md` loads with an `@~/.claude/pstack-models.md` line. pstack has no project-level sheet: its SessionStart hook and skills read only that home path. The repo therefore keeps the M7 roles in [`docs/agents/pstack-models.md`](pstack-models.md), and `scripts/setup-agent-env.sh` copies them to the home path.

The roles use `opus` for every judgment role and `opus, sonnet` for the panel roles. The flow never uses `fable`, except for the last-resort attempt in [Verify and land](../how-to/run-a-milestone.md#verify-and-land).

    scripts/setup-agent-env.sh --dry-run   # print what it would write
    scripts/setup-agent-env.sh             # write the sheet and the include line
    scripts/setup-agent-env.sh --force     # also replace a sheet that differs

The script is idempotent: a second run changes nothing. When `~/.claude/pstack-models.md` already exists and differs from the committed copy, the script prints the diff and exits 1 without writing. Pass `--force` to replace it. To change a role for the team, edit `docs/agents/pstack-models.md` in a PR.

## Skills

Repo skills live in `.claude/skills/`, and `.cursor/skills/` mirrors each one with a symlink. They load with the repo, so a clone needs nothing more. The machine that ran M7 also had six skills in a personal skills folder. The table says which ones the repo now holds and where to get the rest.

| Skill | Decision | Source | Why |
| --- | --- | --- | --- |
| `commit` | Vendored to `.claude/skills/commit` | This repo | It writes the Conventional Commit subject that the PR format check requires. The copy names the check's exact pattern and the owner's `gt modify` flow. |
| `ship-pr` | Not vendored | Personal | [Ship a ticket](../how-to/ship-a-ticket.md) replaces it. It commits with plain git, sets bodies with `gh pr edit` and moves PRs back to draft, which the owner flow forbids. |
| `orchestration` | Listed | `stablyai/orca` | Drives Orca workers. The M7 flow dispatched owners with `monacoctl agents dispatch` and the Agent tool instead. |
| `solana-dev` | Listed | `solana-foundation/solana-dev-skill` | General Solana reference for chain work. Install it with `npx skills add solana-foundation/solana-dev-skill`. |
| `controlling-mobile-devices` | Listed | MobAI | Drives simulators through the MobAI MCP server. `.cursor/skills/ios-simslim-fast-qa` is the repo's QA loop that uses it. |
| `slimming-simulators` | Listed | MobAI (`simslim`) | The `simslim` command line. `just install` offers `simslim` itself. |

The plugins in [Plugins](#plugins) bring their own skills, such as `pstack:poteto-mode`, `superpowers:systematic-debugging` and `compound-engineering:ce-code-review`.
