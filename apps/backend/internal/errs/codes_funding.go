package errs

const (
	CodeOnrampLinkInvalid       Code = "onramp_link_invalid"
	CodeOnrampLinkExpired       Code = "onramp_link_expired"
	CodeOnrampInvalidTransition Code = "onramp_invalid_transition"
)

func fundingRows() map[Code]Row {
	return map[Code]Row{
		CodeOnrampLinkInvalid: {
			Name: "OnrampLinkInvalid", Kind: KindNotFound,
			Message: "This link was already used or isn't valid. Start a new card deposit in the app.",
		},
		CodeOnrampLinkExpired: {
			Name: "OnrampLinkExpired", Kind: KindBlocked,
			Message: "This link has expired. Start a new card deposit in the app.",
		},
		CodeOnrampInvalidTransition: {
			Name: "OnrampInvalidTransition", Kind: KindConflict,
			Message: "This card deposit has already finished.",
		},
	}
}
