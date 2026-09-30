package observability

var (
	AnalyticsCaptureSent    = Msg{Name: "analytics.capture_sent", Required: []string{"event", "uuid"}}
	AnalyticsCaptureSkipped = Msg{Name: "analytics.capture_skipped", Required: []string{"reason"}}
	AnalyticsCaptureFailed  = Msg{
		Name: "analytics.capture_failed", Required: []string{"event", "uuid", "code", "err"},
	}
)
