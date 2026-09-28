package observability

var (
	BusRelayTick          = Msg{Name: "bus.relay.tick", Required: []string{"count", "first_id", "last_id"}}
	BusRelayIdle          = Msg{Name: "bus.relay.idle"}
	BusRelayPublishFailed = Msg{Name: "bus.relay.publish_failed", Required: []string{"code", "err"}}
	BusRelayFailed        = Msg{Name: "bus.relay.failed", Required: []string{"code", "err"}}
	BusDispatched         = Msg{Name: "bus.dispatched", Required: []string{"handler", "subject", "outcome", "code"}}
	BusConsumeError       = Msg{Name: "bus.consume_error", Required: []string{"consumer", "err"}}
	BusRespondFailed      = Msg{Name: "bus.respond_failed", Required: []string{"verdict", "err"}}
	BusDeadLetterDropped  = Msg{
		Name:     "bus.deadletter_dropped",
		Required: []string{"consumer", "msg_id", "err"},
	}
)
