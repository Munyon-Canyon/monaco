package observability

var (
	IdentityWalletSignerMissing = Msg{Name: "identity.wallet.signer_missing", Required: []string{"wallet_id"}}
	IdentityHandleSet           = Msg{Name: "identity.handle.set", Required: []string{"user_id"}}
	IdentityProfileUpdated      = Msg{Name: "identity.profile.updated", Required: []string{"user_id"}}
)
