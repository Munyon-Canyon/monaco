package observability

var (
	NotifyDeviceRegistered   = Msg{Name: "notify.device.registered", Required: []string{"user_id", "environment"}}
	NotifyDeviceUnregistered = Msg{
		Name:     "notify.device.unregistered",
		Required: []string{"user_id", "environment"},
	}
	NotifyDeviceUnregisterSkipped = Msg{Name: "notify.device.unregister_skipped", Required: []string{"user_id"}}
)
