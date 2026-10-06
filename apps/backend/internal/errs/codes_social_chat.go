package errs

const (
	CodeChatBodyInvalid     Code = "chat_body_invalid"
	CodeChatMessageNotFound Code = "chat_message_not_found"
	CodeChatMessageNotOwned Code = "chat_message_not_owned"
	CodeChatParentIsReply   Code = "chat_parent_is_reply"
	CodeChatParentNotFound  Code = "chat_parent_not_found"
)

func (codeFiles) SocialChat() map[Code]Row {
	return map[Code]Row{
		CodeChatBodyInvalid: {
			Name: "ChatBodyInvalid", Kind: KindInvalid, Message: "A message must be 1 to 2000 characters.",
		},
		CodeChatMessageNotFound: {
			Name: "ChatMessageNotFound", Kind: KindNotFound, Message: "We could not find that message.",
		},
		CodeChatMessageNotOwned: {
			Name: "ChatMessageNotOwned", Kind: KindForbidden, Message: "You can only delete your own messages.",
		},
		CodeChatParentIsReply: {
			Name: "ChatParentIsReply", Kind: KindInvalid, Message: "You can only reply to a message in the channel.",
		},
		CodeChatParentNotFound: {
			Name: "ChatParentNotFound", Kind: KindNotFound, Message: "The message you replied to is gone.",
		},
	}
}
