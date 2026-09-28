## Needs from Logan

Every open item across M7, in landing order. Agents tried the CLI, API, subagent, headless `claude -p` and workflow-dispatch routes before listing anything here. This comment is edited in place on each audit tick.

**#456 (CI):** landed on main 2026-09-27 (c971d229). The ruleset (id 24076748) and the fork approval were applied by the root at your request.
- [ ] **Optional:** push a branch that is behind `main` to a PR, and confirm GitHub says it must be updated first.

**#457 (legacy delete):** done. The root removed the gitignored `apps/backend/*.test` binaries from your checkout at your request.

**#464 (NATS, test Postgres, atlas):** done. The root reset the dev DB and applied migration 0001 at your request. After landing, run `just install` in your checkout once to get the pinned atlas and sqlc in `.bin/`.

**#459 (no-comments hooks):**
- [ ] **Optional:** run `cursor-agent login`, which needs a browser. Then run the exact `cursor-agent -p` command in #722's body to see the Cursor agent receive the block message. The Claude Code hook is already proven live.

**#489 (docs site):**
- [ ] **Optional, after landing:** open https://lognorman20.github.io/monaco/ and confirm the site renders. Pages is already enabled (build_type workflow, turned on by the root on 2026-09-27), so the first push to `main` deploys it.

**Milestone-wide:**
- [x] **#789:** verifier GitHub App `monaco-verifier` (ID 5101392) created by the operator and installed on this repo only.
- [ ] **Optional, after landing:** re-enable nightly QA with `gh workflow enable nightly.yml -R lognorman20/monaco`. It was disabled at your request on 2026-09-27.

Status: M7 checkpoint 2 (#786) is on main (6a13e756). The rest of M7 lands on feature branch `backend-rewrite-3`.
- #790 (`just migrate db`, loud schema-behind boot): merged into `backend-rewrite-3` via #791 (108d309e), 2026-09-27.
- #789 PR 1 (#794, ruleset and CI triggers): merged (e55ca161). Ruleset 24089171 now guards `backend-rewrite-3`: `ci / ci-ok` and `verify` (App 5101392) required, up to date, squash only, no direct pushes. No merge queue: GitHub refuses it on a personal-account repo, so PRs land with `gh pr merge --auto --squash`.
- #789 PR 2 (#796 agent guard hook and pr-body.sh, #799 SubagentStop reaper): merged (f7c99549, 1eba0fd7).
- #789 PR 3 (#792 ready job, PR-body and commit-subject lint): merged (4338fce4). The ruleset now also requires the PR format check.
- #789 PR 4 (#797, #798, #804, #800, #805), #807 (bus hang #806 and #801), #808, #809, #810, #811 (CI and landing fixes; verifier App now optional): merged into `backend-rewrite-3` (tip c24356cf). Orchestrator state: branch `m7-rest-state`.
- Paused by the operator before the next wave. Handoff: https://github.com/lognorman20/monaco/issues/492#issuecomment-5861913221
- Next: rest of #789 (retro as CI checks, hooks and `monacoctl agents`), then 479, 480, 481, 482, 483, 484, 486, 490, 491 (partial). #488 stays out.

**Open items: 4** (all optional)

