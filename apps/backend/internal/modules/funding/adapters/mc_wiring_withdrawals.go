package adapters

import (
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/fundingapi"
)

func evmWithdrawalNotImplemented(body api.WithdrawRequest) error {
	evm := body.DestinationKind != nil && *body.DestinationKind == api.WithdrawRequestDestinationKindEvm
	offSolana := body.Network != nil && *body.Network != api.WithdrawRequestNetworkSolana
	if evm || offSolana {
		return errs.New(errs.CodeNotImplemented, "funding.Withdraw")
	}
	return nil
}

func MultichainWithdrawalsWalletActions() multichain.Pending {
	return multichain.Pending{Ticket: "#4972"}
}
