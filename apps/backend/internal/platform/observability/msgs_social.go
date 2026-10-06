package observability

var SocialChatPublishFailed = Msg{
	Name: "social.chat_publish_failed", Required: []string{"message_id", "event", "cause"},
}

var SocialChatAuthorUnreadable = Msg{
	Name: "social.chat_author_unreadable", Required: []string{"message_id", "cause"},
}
