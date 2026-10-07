# Standing orders

These are the rules every agent and person follows in the milestone workflow. They came out of the M7 run, where the operator gave 31 numbered orders during one night and day. This page keeps the ones that still need judgment, one rule each, with the reason and the incident behind it. A dispatch prompt names this page, and the [owner](owner.md) and [verifier](verifier.md) briefs link it.

Hooks and CI checks now enforce many of the original orders, so they are not numbered here. [Enforced by tooling](#enforced-by-tooling) lists them. The Claude Code hooks do not run in a plain terminal, so a person owning a ticket follows that list too. [Superseded](#superseded) lists the orders that no longer apply. The [M7 closeout log](../milestones/m7-closeout.md) records the decisions behind them.

To change a rule, edit this page in a PR and say why under "What came up".

## Orders

1. **Land on `staging`, always through `land-stack`. Only the operator merges into `main`.** Base new work on `staging`. Right after `gt submit` and `scripts/pr-body.sh`, run `monacoctl agents land-stack <top-pr>` once, without asking, unless the user said in the current conversation not to land or queue, then `monacoctl agents watch` under Monitor. If stage 1 is still running, `land-stack` arms the stack, and `agents watch` lands it once stage 1 passes, or prints `armed stack #<top> disarmed: ...` when it fails. The verifier's `verify` status is advisory: landing does not wait on it. It adds `merge-queue` to every PR of the stack, and the Graphite merge queue squashes each PR into one commit on `staging`. Never run `gh pr merge` into `staging`, never change a PR's base, and never add `merge-queue` or `fast-track` by hand. A landed PR shows as closed, not merged. If a stack drops out of the queue, fix it with `gt modify` and `gt submit --stack --no-interactive --draft`, then run `land-stack` again. To change a queued stack, run `monacoctl agents dequeue <top-pr>` first. The agent guard hook blocks `gt submit`, `gt modify`, `gt restack` and `git push` on a stack while any of its PRs carries `merge-queue`. Follow a queued stack with `monacoctl agents watch` under Monitor. Never sleep or poll to wait for it. `main` changes only through a promotion PR from `staging` that the operator merges with a merge commit. [Old flow and new flow](../how-to/ship-a-ticket.md#old-flow-and-new-flow) lists what changed from the checkpoint branches.
    - Why: a promotion carries exactly what landed on `staging`. A PR based on `main` puts commits on `main` that `staging` lacks, and the next promotion must merge them back first.
    - Incident: a CI budget fix (#836) was based on `main`, and `main` then held commits the feature branch lacked. Checkpoint 3 needed a merge-back PR (#892) before it could open.

2. **Work only in your assigned worktree under `.worktrees/`.** `cd` into it first and use absolute paths. Never change the primary checkout.
    - Why: owners run in parallel on one clone. The primary checkout is the operator's build tree and must stay on its branch, clean.
    - Incident: an agent's shell resets its working directory between commands, so a relative path can land in another agent's tree.

3. **Only the root restacks, and one stack at a time.** Update the trunk with `gt sync --no-interactive --no-restack`, then restack the target stack with `gt restack --upstack` in its worktree. Build every branch of the stack afterwards.
    - Why: a restack moves branches that an owner may be building on.
    - Incident: a plain `gt sync` restacked owners' in-progress branches twice (the #479 stack and the #831 F stack). A clean textual restack of #482 still failed to compile, because the tip had renamed a helper.

4. **An owner stops after it arms its stack.** It pushes drafts, runs `scripts/pr-body.sh` for each PR, runs `monacoctl agents land-stack <top-pr>` once, and exits with its PR URLs, heads and decisions. It never waits on CI and never merges.
    - Why: the root and the verifier own everything after the push. A waiting owner holds a lane and burns its context.
    - Incident: the lane cap counts every owner not marked exited. Finished owners that kept running blocked new dispatches until the root ran `monacoctl agents exited`.

5. **A verifier runs on another model, reviews the diff against the ticket and runs no tests.** It reads `gh pr checks` once and fails a PR only for a real defect: wrong behavior, a missing acceptance item the PR claims, a CI-breaking change or a format violation. Its report says exactly what to fix.
    - Why: a second model catches what the owner's model misses. CI already runs the tests.
    - Incident: the #537 verifier caught Jupiter's `priceImpactPct` read as a fraction instead of a percent. It would have refused every real trade.

6. **Use `opus` for owners and `sonnet` for their verifiers. After three failed `opus` attempts on one problem, try `fable` once at medium effort, then park the ticket.** Park with a comment that says what was tried.
    - Why: `opus` and `sonnet` keep cost and behavior predictable. One stronger attempt is cheaper than a parked ticket.
    - Incident: three `opus` owners could not find the #838 NATS flake. One `fable` attempt root-caused it to a race in `nats-server` and fixed it.

7. **Sign every commit and PR body.** Commit messages end with a `Co-Authored-By:` trailer that names the model. PR bodies end with the Claude Code line from the template.
    - Why: a reviewer can tell which model wrote a change and weigh the review for it.
    - Incident: none. This was an order from the start of the run.

8. **Write tests that repeat.** Stamp fixtures relative to the current time, never with a hard-coded date. Fake every external API; never call live Jupiter. Reset global registries and shared servers between runs.
    - Why: the `flake` job reruns each changed test file 20 times, and a date that ages out fails a test months later with no code change.
    - Incident: `TestRegisterLedgerCheck` accumulated entries in a global registry and ejected #904 under the 20 reruns. `TestBusApply` shared one NATS server and failed at `-count=2` (#930).

9. **Say what the PR changes in its title, and record every decision under "What came up".** The last PR of a ticket lists everything only the operator can do under `## Needs from Logan`, or says "Nothing."
    - Why: the operator reads the bodies, not the transcripts. A decision that is not in a body is lost.
    - Incident: the #490 owner left an eval result for the operator to judge. The root judged it and changed the section to "Nothing.", which saved a round trip.

10. **Fix a failed check by its cause.** Rerun only the failed jobs once for an infra flake, with `gh run rerun <run-id> --failed`. Fix a PR format failure caused by the body by editing the body. A fix commit starts its own run, so do not also rerun.
    - Why: a rerun replays the original event, including the body the PR had then.
    - Incident: reruns of #895 and #962 replayed the empty body from when the PR opened and failed again. Docker Hub, Go proxy and `curl` 500 errors ejected #843, #933 and #934, and each landed on requeue with no code change.

11. **Land a fix ahead of the PRs it unblocks.** When a defect on the tip ejects PRs, stop requeueing them. Land the fix first, then requeue the rest behind it.
    - Why: a queued stack tested without the fix fails again.
    - Incident: #931, #933 and #934 were queued ahead of the teardown fix #952. They were dequeued and requeued behind it.

12. **Hand work on through GitHub, not through one machine.** Before you stop, push every branch, keep the draft PR's "What came up" current and post the next step on the ticket. Keep the milestone decision log committed.
    - Why: any engineer must be able to pick up a ticket or a milestone from a fresh clone.
    - Incident: at the M7 closeout the owner records, standing orders and decision log existed only on the operator's machine. The handoff had to tell the next orchestrator to re-dispatch in-flight tickets instead of resuming them.

## Enforced by tooling

These original orders are now enforced. The hooks are `scripts/agent-guard.py`, `scripts/agent-guard-dispatch.py` and `scripts/agent-guard-dev-db.sh`. The checks are the PR format check (`scripts/check-pr-format.py`) and the PR size check (`scripts/check-pr-size.py`).

| Order | Rule | Enforced by |
| --- | --- | --- |
| 3 | Graphite does all branching. No `git rebase` onto a local branch or interactively (`--continue`, `--abort` and a rebase onto a remote-tracking ref such as `origin/staging` are allowed), no `gh pr create`, no `gh pr edit --base`. | agent guard |
| 4, 14, 17, 20 | Template sections, `Part of #N` or `Closes #N` under Why, the "Needs from Logan" section on the last PR, Conventional Commit subjects, a title with no issue number or type prefix, and no SHA that is not an ancestor of the head. | PR format check, agent guard |
| 5, 21 | Each PR is under 1000 changed lines, and no build output is committed. | PR size check |
| 6, 18 | Stage 0 passes on the current tree before a push. Owners and verifiers run no full suite, no `-race` and no local mutation testing. | agent guard |
| 7 | No `gh run watch`, no `gh pr checks --watch` and no `sleep` over 10 seconds. | agent guard |
| 9 | A push never drops commits that the remote has and the local branch lacks. | agent guard |
| 10, 25 | Owner and verifier spawns are `pstack:poteto-agent` on `opus` or `sonnet`, only for a dispatched ticket. A verdict refuses the owner's model and `fable`. | dispatch hook, `monacoctl agents verdict` |
| 13 | Nothing turns durability off on the dev database container. | dev database guard |
| 16 | A plain `gt sync` does not run during a batch. | agent guard |
| 23, 24 | A test package warns at 10 seconds and fails at 20. A laptop run fails past 90 seconds. | `just test backend`, CI |
| 28 | Submit with `gt submit --stack --no-interactive --draft`, then set each PR with `scripts/pr-body.sh`. No `--publish` and no `gh pr ready`. | agent guard |

## Superseded

| Order | What it said | Where it went |
| --- | --- | --- |
| 22 | Operator answers on one morning: a PR already merged, the merge-back token added, Nightly QA left off, `gate-changes` non-blocking. | The token and the non-blocking `gate-changes` warning are in place. The rest is in the [M7 closeout log](../milestones/m7-closeout.md). |
| 26 | Run the gardener nightly as a report issue, without mutation or an auto-PR routine. | Shipped in #905 and #906. See [Gardener](../how-to/gardener.md). |
| 29 | Write docs so a person or an agent can run the workflow. | Shipped as [Ship a ticket](../how-to/ship-a-ticket.md) and [Run a milestone](../how-to/run-a-milestone.md) (#982). |
| 30 | Put the operator's plugins, skills and preferences in the repo. | Shipped as [Agent workflow setup](setup.md) and this page (#983). |
