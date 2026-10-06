package events

func notifyRegistrations() []Registration {
	return []Registration{
		Register[NotifyTestRequested](TypeNotifyTestRequested, 1),
		Register[NotificationSent](TypeNotificationSent, 1),
	}
}
