package adapters

import "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"

func MultichainExecutionWalletActions() multichain.Pending {
	return multichain.Pending{Ticket: "#4970"}
}

func MultichainExecutionEVMWallets() multichain.Pending {
	return multichain.Pending{Ticket: "#4970"}
}
