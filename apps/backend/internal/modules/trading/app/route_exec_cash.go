package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (r *RouteExec) CashLeg(
	ctx context.Context, cabal ids.CabalID, network chain.Network, ledger uint64,
) (multichain.Listing, uint64, error) {
	id, err := sqlc.New(r.d.Layer.d.Reads).CashCarrierListing(ctx, sqlc.CashCarrierListingParams{
		CabalID: cabal.UUID(), Network: string(network),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return multichain.Listing{}, 0, nil
	}
	var listing multichain.Listing
	if err == nil {
		listing, err = r.d.Listings.Listing(ctx, multichain.ListingIDFrom(id))
	}
	var wallet multichain.EVMWallet
	if err == nil {
		wallet, err = r.d.TreasuryEVM.EnsureTreasuryEVMWallet(ctx, cabal)
	}
	if err != nil {
		return multichain.Listing{}, 0, errs.Wrap(err, errs.CodeOf(err), "trading.RouteExec.CashLeg")
	}
	onChain, err := r.d.Cash.Balance(ctx, network, wallet.Address)
	if err != nil {
		return multichain.Listing{}, 0, err
	}
	cash := network.Cash()
	listing.TokenAddress, listing.Decimals, listing.Symbol = cash.Address, cash.Decimals, cash.Symbol
	return listing, min(ledger, onChain.Uint64()), nil
}

func (r *RouteExec) cashLeg(ctx context.Context, b routedBuy) error {
	held, err := r.lockTreasury(ctx, b)
	if err != nil || !held {
		return err
	}
	leg := b.sellLeg()
	step, err := r.d.Journal.Begin(ctx, leg.Ref, leg.Amount)
	hash := step.PrivyActionID
	if err == nil && step.Status == domain.StepPending {
		hash = "cash:" + b.id.String()
		err = r.swap().submit(ctx, leg, hash)
	}
	if err != nil {
		return err
	}
	return r.sold(ctx, b, EVMSwapResult{
		EVMFill: EVMFill{State: FillSucceeded, Out: b.req.InAmount}, Hash: hash,
	})
}
