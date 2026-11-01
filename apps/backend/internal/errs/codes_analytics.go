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
			Message: "Analytics is busy. Try again in a minute.",
		},
		CodePostHogRejected: {
			Name: "PostHogRejected", Kind: KindInternal, Alert: true, Message: "Something went wrong.",
		},
		CodeAnalyticsPII: {
			Name: "AnalyticsPII", Kind: KindInternal, Alert: true, Message: "Something went wrong.",
		},
	}
}
