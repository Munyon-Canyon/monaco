**Milestone:** M7 Backend platform · **Blocked by:** none · **Tracking:** #492

## Context

After pulling checkpoint 2, `just run backend` failed with `boot.stopped err="db.Open: internal"` for both api and worker. The local database was at migration 0001, while the code needs 0002 (`0002_idempotency`, from #476).

## Problem

Three defects made this a debugging session instead of a one-line fix:

1. **No `just` recipe applies migrations.** The only path is `monacoctl migrate apply`, which reads `.atlas-version` and `../../.bin/atlas` relative to the current directory. So it fails from the repo root, and from `scripts/with-dotenv-local.sh`, which changes to the repo root.
2. **The boot error drops its cause.** `checkRevision` returns `errs.New(CodeInternal, op, have, want)`, but `boot.stopped` logs only `err="db.Open: internal"`. The `have` and `want` values never reach the log.
3. **`atlas` is missing after a pull.** It gets installed only when someone runs `just install`.

## Proposal

Implement exactly:

1. `just run backend` never applies migrations. When the database is behind, it keeps failing loudly at boot. That's intended: schema changes are always a deliberate step.
2. Add `just migrate db`. It runs `monacoctl migrate apply` against `.env.local` from any directory, then prints the revision it ended at. If `.bin/atlas` is missing or does not match `.atlas-version`, it fails loudly and names the fix: `run: just install`.
3. `monacoctl migrate` and `bus` resolve `.atlas-version` and `.bin/atlas` from the module root (the directory holding `go.mod`), found by walking up from the current directory. They then work from any directory, including under `with-dotenv-local.sh`.
4. A schema revision mismatch gets its own error code, `db_schema_behind`, instead of `internal`. `boot.stopped` logs the `errs` attributes of the error it stops on, alongside `err`, so the mismatch prints `have=0001 want=0002 hint="run: just migrate db"`.
5. `bus.relay.idle` is logged at most once a minute, not once a second.

## Acceptance Criteria

- [ ] On a database at 0001, `just run backend` exits non-zero. Its `boot.stopped` line shows `db_schema_behind`, `have=0001`, `want=0002` and the hint.
- [ ] `just migrate db` then brings the database to the latest revision, and `just run backend` boots with both `/healthz` endpoints returning 200.
- [ ] `monacoctl migrate apply` works from the repo root, from `apps/backend`, and under `with-dotenv-local.sh`.
- [ ] With `.bin/atlas` removed, `just migrate db` fails and names `just install`.
- [ ] `just run backend` never runs a migration. A test of the recipe script shows it.
- [ ] A 60-second idle run logs `bus.relay.idle` at most twice.

## Verification

    just test backend
    just reset db && just run backend    # expect a loud db_schema_behind failure
    just migrate db && just run backend  # then curl :8080/healthz and :8081/healthz
    (cd /tmp && /path/to/bin/monacoctl migrate apply)

## Done when

Merged into the feature branch. After a fresh pull, `just run backend` either boots or names `just migrate db`, and `just migrate db` fixes it.

**Needs from Logan:**
Nothing.
