---
name: orchestrate-sessions
description: Runs several Claude sessions on one machine as a team that lands milestone PRs on staging all day. One main orchestrator supervises, one dispatcher turns tickets into stacks, one CI manager keeps the pipeline fast. Use when asked to be the main orchestrator over other sessions, to keep sessions unblocked and PRs merging, to maximize PRs landed, or to run the milestones unattended while the user is away.
---

# Orchestrate sessions

Three long-lived sessions share one clone. Each owns one job and never does another's. The commands each step uses are in the **monaco-milestone** skill, and the root's procedure is `docs/how-to/run-a-milestone.md`. This skill says who does what, and how the main orchestrator keeps the other two moving.

| Session | Owns | Never |
| --- | --- | --- |
| Main orchestrator | The milestone order, the exit condition, stall detection, capacity knobs, mobile QA policy, worktree hygiene, the decision log, the list of calls for the user | Writes ticket code, dispatches owners, restacks |
| Dispatcher (the milestone root) | Ranking ready tickets by critical-path value, `agents dispatch`, the stage 0 runners, arming with `land-stack`, restacks, the owner records | Starts an agent with no ranked ticket for it |
| CI manager | `agents watch` and its supervisor, `monacoctl agents` bugs, CI and queue speed, flaky tests, the Xcode and coverage gates | Takes milestone tickets |

Each worker session spawns its own subagents in worktrees under `.worktrees/`. The main orchestrator reaches the other two with `ListAgents` and `SendMessage`, and keeps at most one fix agent of its own, for a blocker no session owns (QA tooling, say).

## Start

1. Get the order from the user as a chain of milestones, for example `M9 => M12 => M11/M10 => M13 & M14`. Send it to the dispatcher. Every dispatch decision follows it.
2. Write the exit condition as something checkable: the ready tickets of the named milestones dispatched and landed in order, no stall over 30 minutes, mobile QA run on every ticket that names it.
3. Start the decision log with the **show-me-your-work** skill. Log one row per decision, landing batch and stall.
4. Record the `origin/staging` SHA. A landing is a new SHA, counted with `git log <last>..origin/staging`. Commit dates lie after a queue squash.
5. Keep the Mac awake for the whole run (`caffeinate -dimsu`). A sleeping Mac freezes the watch, the runners and every check-in.
6. Schedule a check-in every 5 minutes with the `loop` skill, or `CronCreate` when the user is away.

## Each check-in

1. `git fetch origin staging` and count the new landings. Log them as one row.
2. Read the queue and the armed stacks from `bin/monacoctl agents watch --once` and `gh pr list --label merge-queue`.
3. Confirm the watch is alive. Its process exists and its log moved in the last 5 minutes.
4. Check the load (`uptime`) against `[dispatch] max_load` in `.git/.monaco/agents.local.toml`.
5. If nothing landed for 30 minutes, find the cause with the table below and send it to the session that owns it. Name the PR, the evidence and the one action you want.
6. Every hour, compare the open tickets of the current milestones with the owner records. A ready ticket on the critical path with no owner goes to the dispatcher.

Sessions report finished work by message. Read the report and check the claim against `git`, `gh` and the logs before you log it as done.

## Stalls and who fixes them

| Symptom | Cause | Goes to |
| --- | --- | --- |
| Nothing landed, empty queue | Green stacks nobody armed | Dispatcher, to run `land-stack` |
| Stack never merges, CI skipped | The PR's base is `graphite-base/<n>`, or it is still a draft | Dispatcher: dequeue, restack, `gt submit`, `scripts/pr-body.sh`, `land-stack`. Never `gh pr edit --base` |
| `flow <id> changed on staging` or `is in queued stack` | The flows gate | Dispatcher, to restack after the named stack lands. Arm stacks that touch the same flows one at a time |
| Ejected on a conflict | Two stacks edit the same file | Dispatcher restacks the later stack. Ask the CI manager to remove the hotspot, for example by generating the file from per-flow parts |
| Watch missing or its log silent | A hung `gh` call or a crash | CI manager restarts it under the supervisor and fixes the cause |
| Stage 0 over budget, timing failures | Load from too many check slots, or a test that reads the wall clock | Dispatcher drops a slot. CI manager makes the test deterministic. Never raise a budget |
| `staging` itself red (MonacoTests, build) | Parallel PRs that each passed alone | Whoever touched it last. The fix skips every lock and queue ahead of it |
| Lanes full, nothing running | Owner records left over from closed tickets | Dispatcher marks them `exited`. Do this before raising `lanes` |
| Labeled PRs from another machine | A teammate's stack | Leave it alone unless the user hands the tickets over |

Never use `gt sync` while other lanes have unpushed work, because it resets their branches. To move the primary checkout's `staging` forward, run `git switch --detach && git fetch origin staging:staging && git switch staging`.

## Capacity

- A ticket comes first, then an agent. Never start an agent just to fill a lane.
- The load gate (`max_load`) paces dispatch. Don't use `--urgent` to get around it.
- To raise throughput, change `lanes`, `[check] slots` or `max_load` in `.git/.monaco/agents.local.toml`, one at a time. Back the change off if timing failures come back more than twice an hour.
- Back up the file before you change it. Log each change with the load you measured.

## Mobile QA

Tickets whose Done when names mobile QA get a journey run with the **ios-journey-qa** skill. `scripts/qa/journey.py` takes the QA lock and starts and stops the backend itself, so QA always runs with the backend up. Use `--runs 1` while the pipeline is busy. A failing step goes to the dispatcher as a bug ticket, with the step id and its log. Never mark a QA proof as done until the run passed.

## Memory

`journey.py` shuts down the simulators it boots when a run ends, whether it passes, fails or is interrupted. Pass `--keep-sims` only when a person is debugging. Never leave a simulator booted when your turn ends. Run at most 2 simulator lanes at once on a 16 GB Mac. Check with `xcrun simctl list devices booted`.

## Hygiene

After a stack lands, its owner removes its worktree and closes its workspace in the session sidebar (herdr). Check for unpushed commits first, and keep any worktree that has them. Send this as a standing order to every session, so the user can follow the sidebar.

## The user

Keep a list in the decision log, with phase `morning`, of the calls that only the user can make: `gate-change-approved` and `large-pr` labels, secrets, paid services, and taking over a teammate's tickets. Ask once, and keep going around anything blocked.

A status reply leads with the number of PRs landed since the last reply and since the start, then milestone progress, then the open calls.

## Wrap up

When the user says stop:

1. Cancel your own check-in schedule.
2. Tell each session to stop its runners, watch, supervisor, monitors and owners, to start nothing new, and to keep worktrees with unpushed work.
3. Wait for each session's final report. Confirm with `ps` that no runner, `xcodebuild` or backend is left, and that ports 8080 and 8081 are free.
4. Kill orphaned `tail -f` monitor processes whose parent is `1`.
5. Reply with what landed, what each kept worktree holds, and the open calls.
