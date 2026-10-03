package observability

var (
	IdentityWalletSignerMissing = Msg{Name: "identity.wallet.signer_missing", Required: []string{"wallet_id"}}
	IdentityHandleSet           = Msg{Name: "identity.handle.set", Required: []string{"user_id"}}
	IdentityPhoneConflict       = Msg{Name: "identity.phone.conflict", Required: []string{"user_id"}}
	IdentityPhotoPurgeFailed    = Msg{Name: "identity.photo_purge.failed", Required: []string{"user_id"}}
	IdentityProfileUpdated      = Msg{Name: "identity.profile.updated", Required: []string{"user_id"}}
	IdentityXConflict           = Msg{Name: "identity.x.conflict", Required: []string{"user_id"}}

	IdentityFirstDepositBelowThreshold = Msg{
		Name: "identity.first_deposit.below_threshold", Required: []string{"amount_micros", "threshold_micros"},
	}
	IdentityFirstDepositAlreadySet = Msg{Name: "identity.first_deposit.already_set", Required: []string{"user_id"}}
)
