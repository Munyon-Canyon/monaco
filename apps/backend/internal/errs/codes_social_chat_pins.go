package errs

const (
	CodeChatPinForbidden Code = "chat_pin_forbidden"
	CodeChatPinLimit     Code = "chat_pin_limit"
)

func (codeFiles) SocialChatPins() map[Code]Row {
	return map[Code]Row{
		CodeChatPinForbidden: {
			Name: "ChatPinForbidden", Kind: KindForbidden,
			Message: "Only an admin, the author or the one who pinned it can change this pin.",
		},
		CodeChatPinLimit: {
			Name: "ChatPinLimit", Kind: KindConflict, Message: "A cabal can pin up to 3 messages. Unpin one first.",
		},
	}
}
