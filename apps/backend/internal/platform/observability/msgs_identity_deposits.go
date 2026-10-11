package observability

var IdentityDepositAddressesEVMFailed = Msg{
	Name: "identity.deposit_addresses.evm_failed", Required: []string{"user_id", "code"},
}

var IdentityDepositAddressesFeeFailed = Msg{
	Name: "identity.deposit_addresses.fee_failed", Required: []string{"user_id", "code"},
}
