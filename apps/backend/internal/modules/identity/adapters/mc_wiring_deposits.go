package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/identityapi"
)

func (h HTTP) GetMyDepositAddresses(
	context.Context, api.GetMyDepositAddressesRequestObject,
) (api.GetMyDepositAddressesResponseObject, error) {
	return nil, errs.New(errs.CodeNotImplemented, "identity.GetMyDepositAddresses")
}

func MultichainDepositsDepositForwarding() multichain.Pending {
	return multichain.Pending{Ticket: "#4967"}
}
