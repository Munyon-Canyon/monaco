package events

func cabalRegistrations() []Registration {
	return []Registration{
		Register[CabalCreated](TypeCabalCreated, 1),
		Register[CabalMemberJoined](TypeCabalMemberJoined, 1),
		Register[CabalAccessRequested](TypeCabalAccessRequested, 1),
		Register[CabalAccessDecided](TypeCabalAccessDecided, 1),
		Register[CabalMemberLeft](TypeCabalMemberLeft, 1),
		Register[CabalUpdated](TypeCabalUpdated, 1),
	}
}
