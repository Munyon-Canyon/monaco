package events

func socialRegistrations() []Registration {
	return []Registration{
		Register[FollowCreated](TypeFollowCreated, 1),
		Register[FollowRemoved](TypeFollowRemoved, 1),
	}
}
