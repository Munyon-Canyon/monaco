package observability

var SocialChatPublishFailed = Msg{
	Name: "social.chat_publish_failed", Required: []string{"message_id", "event", "cause"},
}
