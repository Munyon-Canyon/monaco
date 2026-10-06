package errs

const (
	CodeCannotFollowSelf     Code = "cannot_follow_self"
	CodeContactHashesInvalid Code = "contact_hashes_invalid"
	CodeFeedItemNotFound     Code = "feed_item_not_found"
	CodeTooManyContactHashes Code = "too_many_contact_hashes"
	CodeUserBanned           Code = "user_banned"
)

func (codeFiles) Social() map[Code]Row {
	return map[Code]Row{
		CodeCannotFollowSelf: {Name: "CannotFollowSelf", Kind: KindInvalid, Message: "You cannot follow yourself."},
		CodeContactHashesInvalid: {
			Name: "ContactHashesInvalid", Kind: KindInvalid, Message: "Those contact hashes are not valid.",
		},
		CodeFeedItemNotFound: {Name: "FeedItemNotFound", Kind: KindNotFound, Message: "This item is not in the feed."},
		CodeTooManyContactHashes: {
			Name: "TooManyContactHashes", Kind: KindInvalid, Message: "Send at most 2000 contact hashes.",
		},
		CodeUserBanned: {Name: "UserBanned", Kind: KindForbidden, Message: "You cannot follow this account."},
	}
}
