package errs

const CodeFeedItemPending Code = "feed_item_pending"

func (codeFiles) SocialFeed() map[Code]Row {
	return map[Code]Row{
		CodeFeedItemPending: {
			Name: "FeedItemPending", Kind: KindUnavailable, Retryable: true,
			Message: "This feed item is still being prepared. Try again in a moment.",
		},
	}
}
