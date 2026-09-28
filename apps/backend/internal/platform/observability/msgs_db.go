package observability

var (
	DBLockLost   = Msg{Name: "db.lock.lost", Required: []string{"lock", "held", "err"}}
	TxCommitted  = Msg{Name: "tx.committed", Required: []string{"event_ids", "attempt"}}
	TxRolledBack = Msg{Name: "tx.rolled_back", Required: []string{"code", "attempt"}}
	TxRetry      = Msg{Name: "tx.retry", Required: []string{"code", "attempt", "delay"}}
)
