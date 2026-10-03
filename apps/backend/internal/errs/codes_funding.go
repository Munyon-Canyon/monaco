package errs

const (
	CodeCabalPaused             Code = "cabal_paused"
	CodeCabalStillPaused        Code = "cabal_still_paused"
	CodeOnrampLinkInvalid       Code = "onramp_link_invalid"
	CodeOnrampLinkExpired       Code = "onramp_link_expired"
	CodeOnrampInvalidTransition Code = "onramp_invalid_transition"
)

func (codeFiles) Funding() map[Code]Row {
	return map[Code]Row{
		CodeCabalPaused: {Name: "CabalPaused", Kind: KindBlocked, Message: "Trading in this cabal is paused."},
		CodeCabalStillPaused: {
			Name: "CabalStillPaused", Kind: KindConflict,
			Message: "The ops pause is lifted, but trading in this cabal is still paused for another reason.",
		},
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
