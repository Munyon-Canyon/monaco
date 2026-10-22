# Land on green log

Record of the land-on-stage-0 experiment described in [ci.md](../ci.md#land-a-pr-on-staging). The setting `[land] on_stage0` ships off. This log starts on 2026-10-22, the day the direct squash and the revert landed (#6304). The operator fills the window rows after turning the setting on.

## Baseline

Measured on 2026-10-18 over 60 landings through the Graphite merge queue.

| Measure | Value |
| --- | --- |
| Defects that reached `staging` | 11 per 100 landings |
| Push to close | 15.8 minutes median, 51.6 minutes p90 |
| Stage 1 on the top PR | 10.2 minutes median |
| Queue draft | 2.6 minutes median |
| Stage 0 | 74 seconds median over 297 passes |

## Window

Start date: not set. Fill each row after the window, from the start date `<date>`.

| Measure | Value | Command |
| --- | --- | --- |
| Landings on stage 0 |  | `gh pr list --state merged --search 'closed:>=<date>' --json number,mergedAt` |
| Real defects that reached `staging` |  | `gh run list --workflow staging.yml --event workflow_dispatch --created '>=<date>'` and read each failed run |
| Reverts opened |  | `gh pr list --state all --search 'head:revert- created:>=<date>'` |
| Minutes from stage 0 start to landed, median and p90 |  | the `passed in` line of `monacoctl agents check` against `mergedAt` of the landed PR |
