package errs

const CodeReactionNotAllowed Code = "reaction_not_allowed"

func (codeFiles) SocialReactions() map[Code]Row {
	return map[Code]Row{
		CodeReactionNotAllowed: {
			Name: "ReactionNotAllowed", Kind: KindForbidden, Message: "You cannot react to this item.",
		},
	}
}
