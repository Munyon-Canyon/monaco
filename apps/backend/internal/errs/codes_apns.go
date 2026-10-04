package errs

const (
	CodeAPNSUnavailable Code = "apns_unavailable"
	CodeAPNSAuthFailed  Code = "apns_auth_failed"
)

func (codeFiles) Apns() map[Code]Row {
	return map[Code]Row{
		CodeAPNSUnavailable: {
			Name: "APNSUnavailable", Kind: KindUnavailable, Retryable: true,
			Message: "The push service is unavailable. Try again shortly.",
		},
		CodeAPNSAuthFailed: {
			Name: "APNSAuthFailed", Kind: KindInternal, Alert: true,
			Message: "Something went wrong.",
		},
	}
}
