# Right calls log

Dated record of changes to [right-calls.md](../right-calls.md). Add one line per change, newest last.

- 2026-10-28: Added the calls store and the rule: `calls`, `call_voters` and `cabal_calls`, the `ranking_calls` consumer, and `ranking/port.Calls`. The call columns use `is_right`, since `right` is a reserved word in Postgres (#6434).
