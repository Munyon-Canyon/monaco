package adapters

import "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"

func MultichainValuationEVMPriceSource() multichain.Pending {
	return multichain.Pending{Ticket: "#4971"}
}
