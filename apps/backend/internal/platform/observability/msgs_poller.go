package observability

var (
	PollerTick    = Msg{Name: "poller.tick", Required: []string{"poller", "scanned", "changed", "duration_ms"}}
	PollerFailed  = Msg{Name: "poller.tick.failed", Required: []string{"poller", "code", "err", "alert"}}
	PollerCrashed = Msg{Name: "poller.tick.crashed", Required: []string{"poller", "code"}}
	PollerSkipped = Msg{Name: "poller.tick.skipped_locked", Required: []string{"poller"}}
)
