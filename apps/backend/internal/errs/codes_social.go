package errs

const (
	CodeCannotFollowSelf Code = "cannot_follow_self"
	CodeFeedItemNotFound Code = "feed_item_not_found"
	CodeUserBanned       Code = "user_banned"
)

func (codeFiles) Social() map[Code]Row {
	return map[Code]Row{
		CodeCannotFollowSelf: {Name: "CannotFollowSelf", Kind: KindInvalid, Message: "You cannot follow yourself."},
		CodeFeedItemNotFound: {Name: "FeedItemNotFound", Kind: KindNotFound, Message: "This item is not in the feed."},
		CodeUserBanned:       {Name: "UserBanned", Kind: KindForbidden, Message: "You cannot follow this account."},
	}
}
