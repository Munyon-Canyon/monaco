package observability

var IdentityDepositAddressesEVMFailed = Msg{
	Name: "identity.deposit_addresses.evm_failed", Required: []string{"user_id", "code"},
}
