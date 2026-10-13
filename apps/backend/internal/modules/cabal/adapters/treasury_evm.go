package adapters

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type TreasuryEVMWallets struct {
	DB      sqlc.DBTX
	Wallets multichain.EVMWallets
	Clock   clock.Clock
}

func (t TreasuryEVMWallets) EnsureTreasuryEVMWallet(
	ctx context.Context,
	cabalID ids.CabalID,
) (multichain.EVMWallet, error) {
	const op = "cabal.EnsureTreasuryEVMWallet"
	if got, found, err := t.stored(ctx, cabalID); err != nil || found {
		return got, err
	}
	wallet, err := t.Wallets.CreateAppEVMWallet(ctx, "treasury-evm:"+cabalID.String())
	if err != nil {
		return multichain.EVMWallet{}, err
	}
	if err := sqlc.New(t.DB).InsertTreasuryEVMWallet(ctx, sqlc.InsertTreasuryEVMWalletParams{
		CabalID: cabalID.UUID(), PrivyWalletID: wallet.ID, Address: wallet.Address.String(),
		CreatedAt: t.Clock.Now().UTC(),
	}); err != nil {
		return multichain.EVMWallet{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	got, found, err := t.stored(ctx, cabalID)
	if err == nil && !found {
		return multichain.EVMWallet{}, errs.New(errs.CodeInternal, op)
	}
	return got, err
}

func (t TreasuryEVMWallets) stored(ctx context.Context, cabalID ids.CabalID) (multichain.EVMWallet, bool, error) {
	const op = "cabal.TreasuryEVMWallets.stored"
	row, err := sqlc.New(t.DB).FindTreasuryEVMWallet(ctx, cabalID.UUID())
	if errors.Is(err, pgx.ErrNoRows) {
		return multichain.EVMWallet{}, false, nil
	}
	if err != nil {
		return multichain.EVMWallet{}, false, errs.Wrap(err, errs.CodeInternal, op)
	}
	addr, err := chain.ParseEVMAddress(row.Address)
	if err != nil {
		return multichain.EVMWallet{}, false, errs.Wrap(err, errs.CodeInternal, op)
	}
	return multichain.EVMWallet{ID: row.PrivyWalletID, Address: addr}, true, nil
}
