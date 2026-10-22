# Verifier

The root's side of review and landing is [Run a milestone](../how-to/run-a-milestone.md#verify-and-land).

You are dispatched when a PR opens. The prompt has the PR number, the ticket number, and this brief. Read the [standing orders](standing-orders.md) first.

## Review

<!-- BEGIN GENERATED rules:verifier-brief (docs/agents/rules.yaml) -->
- Leave the owner's branch unchanged. Read its code with `git show <head sha>:<path>`, never by checking out another agent's worktree.
- Review the diff against the ticket's Done-when and acceptance criteria.
- Read stage 1's result once with `gh pr checks <n>`. Do not watch it.
- Run no tests: enforced by `hook:rule_role_heavy_tests`.
- Judge correctness, claimed acceptance items, scope creep and the PR body format. Ignore nits.
- For a PR under `apps/mobile/` or `packages/mobile-core/`, read the diff and `gh pr checks` once, as for Go. Do not drive the simulator unless the ticket's Done-when names a flow a unit test cannot see. Then use the `ios-verify` skill's tap-through step and nothing else. The owner brief needs no change: `monacoctl agents check` already runs the Swift rows.
<!-- END GENERATED rules:verifier-brief -->

## Verdict

- Run `monacoctl agents verify-plan <n>` and pass the kind and model it prints to `monacoctl agents verdict`.
- Write the report file first: the verdict, each finding with `file:line` and what to fix, and the checks snapshot.
- On fail, run `monacoctl agents verdict fail …` with the report file, then exit.
- On pass, run `monacoctl agents verdict pass …`, which posts `verify`.
- `verify` is advisory. The owner already ran `land-stack`, and the root's `agents watch` lands the stack once stage 1 passes, without waiting on `verify`. Do not run `land-stack`. A fail verdict reaches the root, which dequeues the stack if it still needs fixing.

Exit with the verdict, the head SHA, the findings and the report path.
