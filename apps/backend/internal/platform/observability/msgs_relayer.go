package observability

var (
	RelayerSOLToppedUp      = Msg{Name: "relayer.sol_topped_up", Required: []string{"wallet", "lamports", "signature"}}
	RelayerTopUpUnderfunded = Msg{
		Name:     "relayer.topup_underfunded",
		Required: []string{"relayer", "wallet", "relayer_lamports", "lamports", "floor_lamports"},
	}
	RelayerBootUnderfunded = Msg{
		Name:     "relayer.boot_underfunded",
		Required: []string{"relayer", "floor_lamports", "error"},
	}
	RelayerTopUpCapped = Msg{Name: "relayer.topup_capped", Required: []string{"wallet", "topups"}}
	RelayerCoSigned    = Msg{
		Name: "relayer.cosigned",
		Required: []string{
			"priority_fee_lamports", "base_fee_lamports", "ata_creations", "rent_lamports", "total_lamports",
			"fee_cap_lamports", "rent_cap_lamports",
		},
	}
)
