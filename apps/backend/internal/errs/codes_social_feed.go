package errs

const (
	CodeFeedItemPending       Code = "feed_item_pending"
	CodeFeedFillBatchMismatch Code = "feed_fill_batch_mismatch"
)

func (codeFiles) SocialFeed() map[Code]Row {
	return map[Code]Row{
		CodeFeedItemPending: {
			Name: "FeedItemPending", Kind: KindUnavailable, Retryable: true,
			Message: "This feed item is still being prepared. Try again in a moment.",
		},
		CodeFeedFillBatchMismatch: {
			Name: "FeedFillBatchMismatch", Kind: KindInternal, Alert: true, Message: "Something went wrong.",
		},
	}
}
