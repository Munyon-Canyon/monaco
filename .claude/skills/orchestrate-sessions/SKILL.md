---
name: orchestrate-sessions
description: Runs several Claude sessions on one machine as a team that lands milestone PRs on staging all day. One main orchestrator supervises, one dispatcher turns tickets into stacks, one CI manager keeps local stage 0 and CI fast so agents get PRs up with the least wait. Use when asked to be the main orchestrator over other sessions, to be the CI manager or CI optimization agent, to keep sessions unblocked and PRs merging, to maximize PRs landed, or to run the milestones unattended while the user is away.
---

# Orchestrate sessions

Three long-lived sessions share one clone. Each owns one job and never does another's. The commands each step uses are in the **monaco-milestone** skill, and the root's procedure is `docs/how-to/run-a-milestone.md`. This skill says who does what, and how the main orchestrator keeps the other two moving.

| Session | Owns | Never |
| --- | --- | --- |
| Main orchestrator | The milestone order, the exit condition, stall detection, capacity knobs, mobile QA policy, worktree hygiene, the decision log, the list of calls for the user | Writes ticket code, dispatches owners, restacks |
| Dispatcher (the milestone root) | Ranking ready tickets by critical-path value, `agents dispatch`, the stage 0 runners, arming with `land-stack`, restacks, the owner records | Starts an agent with no ranked ticket for it |
| CI manager | Throughput and latency of stage 0 and CI: the machine's CPU, test databases, disk and caches, the stage 0 slots and build locks, `agents watch`, `monacoctl agents` bugs, flaky and load-sensitive tests, the Xcode and coverage gates | Takes milestone tickets |

Each worker session spawns its own subagents in worktrees under `.worktrees/`. The main orchestrator reaches the other two with `ListAgents` and `SendMessage`, and keeps at most one fix agent of its own, for a blocker no session owns (QA tooling, say).

## Start

1. Get the order from the user as a chain of milestones, for example `M9 => M12 => M11/M10 => M13 & M14`. Send it to the dispatcher. Every dispatch decision follows it.
2. Write the exit condition as something checkable: the ready tickets of the named milestones dispatched and landed in order, no stall over 15 minutes, mobile QA run on every ticket that names it.
3. Start the decision log with the **show-me-your-work** skill. Log one row per decision, landing batch and stall.
4. Record the `origin/staging` SHA. A landing is a new SHA, counted with `git log <last>..origin/staging`. Commit dates lie after a queue squash.
5. Keep the Mac awake for the whole run (`caffeinate -dimsu`). A sleeping Mac freezes the watch, the runners and every check-in.
6. Schedule a check-in every 5 minutes with the `loop` skill, or `CronCreate` when the user is away.

## Each check-in

1. `git fetch origin staging` and count the new landings. Log them as one row.
2. Read the queue and the armed stacks from `bin/monacoctl agents watch --once` and `gh pr list --label merge-queue`.
3. Confirm the watch is alive. Its process exists and its log moved in the last 5 minutes.
4. Read the machine the way the CI manager does (below): CPU idle, test-database CPU, memory, disk free, check queue depth and stage 0 failures by cause.
5. If nothing landed for 15 minutes, find the cause with the table below and send it to the session that owns it. Name the PR, the evidence and the one action you want.
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
| Every queued check dies at once with `wait for a stage 0 slot: context canceled` | A pattern `pkill`/`killall`, or an agent that returned while its check waited | CI manager finds the command in the transcripts and the owner. Lanes requeue |
| Stage 0 frozen while load is low | Running checks wait on the Xcode lock behind a hung or orphaned build | CI manager names the holder (`/private/tmp/monaco-xcodebuild.lock/pid`). Its owner kills it by pid |
| Disk under 50 GB free | Go build cache and per-worktree DerivedData, `.build` and simulators | CI manager runs the cleanup below |
| `staging` itself red (MonacoTests, build) | Parallel PRs that each passed alone | Whoever touched it last. The fix skips every lock and queue ahead of it |
| Lanes full, nothing running | Owner records left over from closed tickets | Dispatcher marks them `exited`. Do this before raising `lanes` |
| Labeled PRs from another machine | A teammate's stack | Leave it alone unless the user hands the tickets over |

Never use `gt sync` while other lanes have unpushed work, because it resets their branches. To move the primary checkout's `staging` forward, run `git switch --detach && git fetch origin staging:staging && git switch staging`.

## Capacity

- A ticket comes first, then an agent. Never start an agent just to fill a lane.
- Stage 0 capacity is what's short, not dispatch. The check queue, `[check] slots` and the `xcode`/`swiftpm` locks pace the heavy work. Dispatching an agent costs almost no CPU, so keep `[dispatch] max_load` high (300) and don't let it block dispatch.
- To raise throughput, change `lanes`, `[check] slots` or `xcode-slots` in `.git/.monaco/`, one at a time, and only after the CI manager's numbers say the machine has room. Back the change off if timing failures pass one every 10 minutes.
- A check that is already waiting keeps the slot count it read at start. After raising slots, restart the waiting checks, or they hold the head of the line at the old count.
- Back up the file before you change it. Log each change with what you measured.

## The CI manager

The goal is the most PRs up and landed per hour, with the least time from a lane's commit to `staging`. Latency adds up across the stage 0 queue wait, the check run, reruns after failures, CI stage 1 and the merge queue. Measure each before changing anything, and log the before and after.

Each check-in, measure:

- CPU with `top -l 2 -n 0 -s 2`, not `uptime`. On macOS the load average also counts processes waiting on disk, so load 300 can mean an idle CPU.
- Test-database CPU with `docker stats --no-stream`, and memory with `memory_pressure`.
- Disk free on `/System/Volumes/Data`.
- Queue depth (`.git/.monaco/check-queue`), and the waiters on the `xcode` and `swiftpm` locks (`/private/tmp/monaco-*.lock.queue`).
- Stage 0 failures of the last hour from `.git/pstack/m8/logs/check-*.log`. Sort each into timing, database down or a real defect, and read the failing line before you call it one or the other.

Pull these levers in order, cheapest win first:

1. Cut the work per check. Run only the packages that import a change. Share build caches across worktrees. Keep repeated work, like per-test probes, out of the test database's hot path.
2. Cut the reruns. Every stage 0 failure sends a lane to the back of the queue. Make a load-sensitive test assert the work it guards, or wait for a required event, and prove it still fails on the defect. Never raise a budget.
3. Then raise slots, one step at a time, by the Capacity rules.

Rules that cost a day of throughput to learn:

- Never `pkill` or `killall` by pattern. One pattern kill cancels every lane's queued checks and the shared watch. Kill a pid after `lsof -a -p <pid> -d cwd` shows your own worktree. The agent guard blocks the rest.
- Only a session runs `monacoctl agents check`, in the background (`nohup ... &`) and never in a foreground tool call, whose 10-minute cap kills it. A subagent that returns while its check waits loses the check to the reaper. Restate both rules every time you brief or resume a coder.
- A stacked PR must pass its check without the PR above it. If two PRs each need the other's fix, fold them into one PR.
- `agents batch` replaces the whole batch. Read `.git/pstack/<milestone>/batch.json` and include the tickets already there.
- Size Docker Desktop's memory to the test databases: slot 0 has a 4 GB tmpfs and each other slot 1.5 GB, plus headroom (32 GB on a 64 GB Mac). After a Docker restart, start the dev `monaco-postgres` and `monaco-nats` by hand, since they have no restart policy.
- Keep at least 50 GB of disk free. Every hour, delete Go build cache files not modified in 24 hours (`find "$(go env GOCACHE)" -type f -mtime +1 -delete`), plus the `.build/DerivedData`, `packages/mobile-core/.build` and shut-down `Monaco <lane>` simulator of every landed worktree with no unpushed or uncommitted work. Never `simctl erase`.
- In CI, rerun only failed jobs for a flake, and raise a job timeout (not a test gate) when a job is cancelled while still compiling. Name each hotspot that serializes stacks to the main orchestrator.

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
