package observability

var CabalTreasuryEVMWalletSkipped = Msg{
	Name: "cabal.treasury_evm_wallet.skipped", Required: []string{"cabal_id", "code"},
}
