package events

func socialRegistrations() []Registration {
	return []Registration{
		Register[ChatMessagePosted](TypeChatMessagePosted, 1),
		Register[FollowCreated](TypeFollowCreated, 1),
		Register[FollowRemoved](TypeFollowRemoved, 1),
	}
}
