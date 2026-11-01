package app

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const routedReceiptsOp = "trading.RoutedReceipts"

type EVMReceipts interface {
	Inspect(ctx context.Context, network chain.Network, txHash string, out, owner chain.EVMAddress) (EVMFill, error)
}

type RoutedReceiptReader interface {
	Receipt(ctx context.Context, swap ids.SwapID) (EVMFill, error)
}

type RoutedReceipts struct {
	Reads    sqlc.DBTX
	Listings Listings
	Wallets  TreasuryEVM
	Venue    EVMReceipts
}

func (r RoutedReceipts) Receipt(ctx context.Context, swap ids.SwapID) (EVMFill, error) {
	q := sqlc.New(r.Reads)
	step, err := readStep(ctx, q, swap, domain.StepSwap)
	if errs.CodeOf(err) == errs.CodeNotFound || err == nil && step.PrivyActionID == "" {
		return EVMFill{State: FillPending}, nil
	}
	if err != nil {
		return EVMFill{}, errs.Wrap(err, errs.CodeInternal, routedReceiptsOp, slog.String("swap_id", swap.String()))
	}
	row, err := q.RoutedSwap(ctx, swap.UUID())
	var comparison multichain.Comparison
	if err == nil {
		err = json.Unmarshal(row.Comparison, &comparison)
	}
	if err != nil {
		return EVMFill{}, errs.Wrap(err, errs.CodeInternal, routedReceiptsOp, slog.String("swap_id", swap.String()))
	}
	listing, err := r.Listings.Listing(ctx, comparison.Winner)
	if err != nil {
		return EVMFill{}, err
	}
	wallet, err := r.Wallets.EnsureTreasuryEVMWallet(ctx, ids.CabalIDFrom(row.CabalID))
	if err != nil {
		return EVMFill{}, err
	}
	out, err := receivedToken(listing, domain.Action(row.Action))
	if err != nil {
		return EVMFill{}, err
	}
	return r.Venue.Inspect(ctx, step.Network, step.PrivyActionID, out, wallet.Address)
}

func receivedToken(listing multichain.Listing, action domain.Action) (chain.EVMAddress, error) {
	address := listing.TokenAddress
	if action == domain.ActionSell {
		address = listing.Network.Cash().Address
	}
	token, err := chain.ParseEVMAddress(address)
	if err != nil {
		return chain.EVMAddress{}, errs.Wrap(err, errs.CodeDecodeFailed, routedReceiptsOp)
	}
	return token, nil
}
