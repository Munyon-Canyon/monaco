package errs

const (
	CodeCommentMembersOnly    Code = "comment_members_only"
	CodeCommentNotAuthor      Code = "comment_not_author"
	CodeCommentNotFound       Code = "comment_not_found"
	CodeCommentParentMismatch Code = "comment_parent_mismatch"
)

func (codeFiles) SocialComments() map[Code]Row {
	return map[Code]Row{
		CodeCommentMembersOnly: {
			Name: "CommentMembersOnly", Kind: KindForbidden,
			Message: "Only members of this cabal can comment on its proposals.",
		},
		CodeCommentNotAuthor: {
			Name: "CommentNotAuthor", Kind: KindForbidden, Message: "You can only delete your own comments.",
		},
		CodeCommentNotFound: {
			Name: "CommentNotFound", Kind: KindNotFound, Message: "We could not find that comment.",
		},
		CodeCommentParentMismatch: {
			Name: "CommentParentMismatch", Kind: KindInvalid,
			Message: "The comment you replied to is not on this item.",
		},
	}
}
