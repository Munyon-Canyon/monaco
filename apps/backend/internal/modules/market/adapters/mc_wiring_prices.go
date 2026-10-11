package adapters

import "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"

func MultichainPricesEVMPriceSource() multichain.Pending {
	return multichain.Pending{Ticket: "#4971"}
}
