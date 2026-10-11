package adapters

import "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"

func MultichainDepositsDepositForwarding() multichain.Pending {
	return multichain.Pending{Ticket: "#4967"}
}

func MultichainDepositsEVMDepositWallets() PendingDepositWallets {
	return PendingDepositWallets{Ticket: "#4966"}
}
