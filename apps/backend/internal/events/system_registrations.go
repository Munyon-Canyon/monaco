package events

func systemRegistrations() []Registration {
	return []Registration{
		Register[SystemPinged](TypeSystemPinged, 1),
		Register[SystemPingFlagged](TypeSystemPingFlagged, 1),
	}
}
