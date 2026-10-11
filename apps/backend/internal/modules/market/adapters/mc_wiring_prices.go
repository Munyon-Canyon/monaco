package adapters

import (
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/evmprices"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

func MultichainPricesEVMPriceSource(quoter multichain.Quoter) evmprices.Source {
	return evmprices.New(quoter)
}
