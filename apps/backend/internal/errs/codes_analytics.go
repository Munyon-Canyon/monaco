package errs

const (
	CodePostHogUnavailable Code = "post_hog_unavailable"
	CodePostHogRejected    Code = "post_hog_rejected"
	CodeAnalyticsPII       Code = "analytics_pii"
)

func (codeFiles) Analytics() map[Code]Row {
	return map[Code]Row{
		CodePostHogUnavailable: {
			Name: "PostHogUnavailable", Kind: KindUnavailable, Retryable: true,
			Message: "The analytics provider is unavailable. Try again shortly.",
		},
		CodePostHogRejected: {
			Name: "PostHogRejected", Kind: KindInternal, Alert: true, Message: "Something went wrong.",
		},
		CodeAnalyticsPII: {
			Name: "AnalyticsPII", Kind: KindInternal, Alert: true, Message: "Something went wrong.",
		},
	}
}
