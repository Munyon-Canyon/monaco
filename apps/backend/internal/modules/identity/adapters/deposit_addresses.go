package adapters

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	baseapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/identityapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type DepositWallets interface {
	EnsureEVMDepositWallet(ctx context.Context, user ids.UserID) (chain.EVMAddress, error)
}

type MemberWallets interface {
	MemberWallet(ctx context.Context, user ids.UserID) (port.MemberWallet, error)
}

type DepositAddresses struct {
	Members  MemberWallets
	EVM      DepositWallets
	Networks []chain.Network
}

type PendingDepositWallets struct{ Ticket string }

func (p PendingDepositWallets) EnsureEVMDepositWallet(context.Context, ids.UserID) (chain.EVMAddress, error) {
	return chain.EVMAddress{}, errs.New(errs.CodeNotImplemented, "identity.EnsureEVMDepositWallet",
		slog.String("ticket", p.Ticket))
}

func (h HTTP) GetMyDepositAddresses(
	ctx context.Context, _ api.GetMyDepositAddressesRequestObject,
) (api.GetMyDepositAddressesResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	addresses, err := h.Deposits.list(ctx, user)
	if err != nil {
		return nil, err
	}
	return api.GetMyDepositAddresses200JSONResponse(api.DepositAddresses{Addresses: addresses}), nil
}

func (d DepositAddresses) list(ctx context.Context, user ids.UserID) ([]api.DepositAddress, error) {
	wallet, err := d.Members.MemberWallet(ctx, user)
	if err != nil {
		return nil, err
	}
	out := []api.DepositAddress{depositAddress(chain.NetworkSolana, string(wallet.Address))}
	if len(d.Networks) == 0 {
		return out, nil
	}
	evm, err := d.EVM.EnsureEVMDepositWallet(ctx, user)
	if err != nil {
		observability.Degraded(ctx, observability.IdentityDepositAddressesEVMFailed,
			slog.String("user_id", user.String()), slog.String("code", string(errs.CodeOf(err))))
		return out, nil
	}
	for _, network := range d.Networks {
		out = append(out, depositAddress(network, evm.String()))
	}
	return out, nil
}

func depositAddress(network chain.Network, address string) api.DepositAddress {
	return api.DepositAddress{Network: baseapi.Network(network), Address: address, Asset: api.USDC}
}
