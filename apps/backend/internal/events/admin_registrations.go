package events

func adminRegistrations() []Registration {
	return []Registration{
		Register[AdminGranted](TypeAdminGranted, 1),
		Register[AdminRevoked](TypeAdminRevoked, 1),
	}
}
