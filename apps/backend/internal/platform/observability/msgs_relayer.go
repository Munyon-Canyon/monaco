package observability

var (
	RelayerSOLToppedUp      = Msg{Name: "relayer.sol_topped_up", Required: []string{"wallet", "lamports", "signature"}}
	RelayerTopUpUnderfunded = Msg{
		Name:     "relayer.topup_underfunded",
		Required: []string{"relayer", "wallet", "relayer_lamports", "lamports", "floor_lamports"},
	}
)
