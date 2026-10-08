package events

func socialRegistrations() []Registration {
	return []Registration{
		Register[BlockCreated](TypeBlockCreated, 1),
		Register[BlockRemoved](TypeBlockRemoved, 1),
		Register[ChatMessagePosted](TypeChatMessagePosted, 1),
		Register[CommentCreated](TypeCommentCreated, 1),
		Register[CommentDeleted](TypeCommentDeleted, 1),
		Register[FollowCreated](TypeFollowCreated, 1),
		Register[FollowRemoved](TypeFollowRemoved, 1),
		Register[ReportCreated](TypeReportCreated, 1),
	}
}
