---
name: ios-verify
description: Verify the Monaco iOS app. Use when asked to verify the app, run MonacoTests, or check the iOS build.
---

# Verify the iOS app

1. Run `just test mobile` first. That is host `swift test` in `packages/mobile-core`, plus the lint from #938.
2. Run `monacoctl agents check` from `apps/backend` (`go run ./cmd/monacoctl agents check`). Read the xcsift TOON output in the check log.
3. When a `MonacoTests` test fails, rerun only that test with `-only-testing:MonacoTests/<Class>/<test>`.

Tap through the simulator only when the ticket's Done-when names a visual change or a flow a unit test cannot see. Then use `.cursor/skills/ios-simslim-fast-qa/SKILL.md` on the simulator from `scripts/gold-sim-udid.sh`: your worktree's own lane simulator, or the gold one in the primary checkout. Never boot a new simulator for unit tests. Never `simctl erase`.
