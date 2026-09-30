package observability

var (
	APNSNoopSend     = Msg{Name: "apns.noop_send", Required: []string{"user_id"}}
	BootPushDisabled = Msg{Name: "boot.push_disabled", Required: []string{"service", "reason"}}
)
