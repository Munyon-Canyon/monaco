package observability

var IdentityWalletSignerMissing = Msg{Name: "identity.wallet.signer_missing", Required: []string{"wallet_id"}}

var IdentityProfileUpdated = Msg{Name: "identity.profile.updated", Required: []string{"user_id"}}
