package adapters

import (
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

func evmWithdrawalNotImplemented(r domain.WithdrawalRequest) error {
	if r.Kind() == domain.DestinationEVM {
		return errs.New(errs.CodeNotImplemented, "funding.Withdraw")
	}
	return nil
}

func MultichainWithdrawalsWalletActions() multichain.Pending {
	return multichain.Pending{Ticket: "#4972"}
}
