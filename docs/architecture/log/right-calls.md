# Right calls log

Dated record of changes to [right-calls.md](../right-calls.md). Add one line per change, newest last.

- 2026-10-28: The store and the rule shipped (#6434). `call_voters."right"` and `cabal_calls."right"` are quoted because `right` is a reserved word in Postgres. A call with no `kind` is never counted, which keeps stray rows from rule proposals out of every record.
