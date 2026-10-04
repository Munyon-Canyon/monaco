package testkit

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const USDCMint chain.SolanaAddress = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"

func Config() config.Config { return config.Config{Solana: config.Solana{USDCMint: string(USDCMint)}} }

type Ledger struct {
	t      SeedT
	uow    *db.UnitOfWork
	ledger app.Ledger
	ids    ids.Generator
}

func NewLedger(t SeedT, pool *pgxpool.Pool) *Ledger {
	t.Helper()
	return &Ledger{
		t: t, uow: db.New(pool, ids.Real{}, clock.Real{}), ids: ids.Real{},
		ledger: app.NewLedger(USDCMint, clock.Real{}),
	}
}

func (l *Ledger) WithFundedMember(user ids.UserID, cabal ids.CabalID, micros money.Micros) *Ledger {
	l.t.Helper()
	l.do(func(ctx context.Context, tx db.Tx) error {
		if err := l.ledger.LockCabal(ctx, tx, cabal); err != nil {
			return err
		}
		var total, pot money.Micros
		if err := tx.Queries().QueryRow(ctx, `SELECT
			(SELECT coalesce(sum(share_units), 0)::text FROM user_positions WHERE cabal_id = $1),
			(SELECT coalesce(sum(cost_basis_micros), 0)::text FROM cabal_positions WHERE cabal_id = $1)`,
			cabal.UUID()).Scan(&total, &pot); err != nil {
			return errs.Wrap(err, errs.CodeInternal, "testkit.Ledger.WithFundedMember")
		}
		minted, err := domain.MintShares(micros, money.SharesUnitsFromUint64(total.Uint64()), pot)
		if err != nil {
			return err
		}
		deposit, fund, inflow, err := l.fundTxns(user, cabal, micros.Uint64(), minted.Uint64())
		if err != nil {
			return err
		}
		if err := l.ledger.PostUserTxn(ctx, tx, deposit); err != nil {
			return err
		}
		if err := l.ledger.PostUserTxn(ctx, tx, fund); err != nil {
			return err
		}
		return l.ledger.PostCabalTxn(ctx, tx, inflow)
	})
	return l
}

func (l *Ledger) fundTxns(
	user ids.UserID, cabal ids.CabalID, micros, units uint64,
) (domain.UserTxn, domain.UserTxn, domain.CabalTxn, error) {
	usdc, shares, transfer := domain.MintAsset(USDCMint), domain.SharesAsset(cabal), l.ids.NewV7()
	in, out := l.signed(micros)
	minted, burned := l.signed(units)
	deposit, err := domain.NewUserTxn(domain.UserTxnHeader{
		ID: l.ids.NewV7(), UserID: user, Kind: domain.UserDeposit, Status: domain.TxnSettled,
	}, []domain.UserEntry{
		{Account: domain.UserExternal, Asset: usdc, Amount: out},
		{Account: domain.UserWallet, Asset: usdc, Amount: in},
	})
	if err != nil {
		return domain.UserTxn{}, domain.UserTxn{}, domain.CabalTxn{}, err
	}
	fund, err := domain.NewUserTxn(domain.UserTxnHeader{
		ID: l.ids.NewV7(), UserID: user, CabalID: cabal, Kind: domain.UserFund, Status: domain.TxnSettled,
		TransferID: transfer,
	}, []domain.UserEntry{
		{Account: domain.UserWallet, Asset: usdc, Amount: out},
		{Account: domain.UserCabal, Asset: usdc, Amount: in},
		{Account: domain.UserHolder, Asset: shares, Amount: minted},
		{Account: domain.UserIssuer, Asset: shares, Amount: burned},
	})
	if err != nil {
		return domain.UserTxn{}, domain.UserTxn{}, domain.CabalTxn{}, err
	}
	inflow, err := domain.NewCabalTxn(domain.CabalTxnHeader{
		ID: l.ids.NewV7(), CabalID: cabal, Kind: domain.CabalFund, Status: domain.TxnSettled, TransferID: transfer,
	}, []domain.CabalEntry{
		{Account: domain.CabalMembers, Asset: usdc, Amount: out},
		{Account: domain.CabalTreasury, Asset: usdc, Amount: in},
	})
	return deposit, fund, inflow, err
}

func (l *Ledger) WithHolding(cabal ids.CabalID, mint chain.SolanaAddress, units money.BaseUnits) *Ledger {
	l.t.Helper()
	in, out := l.signed(units.Uint64())
	asset := domain.MintAsset(mint)
	txn, err := domain.NewCabalTxn(domain.CabalTxnHeader{
		ID: l.ids.NewV7(), CabalID: cabal, Kind: domain.CabalSwap, Status: domain.TxnSettled, SwapID: l.ids.NewV7(),
	}, []domain.CabalEntry{
		{Account: domain.CabalVenue, Asset: asset, Amount: out},
		{Account: domain.CabalTreasury, Asset: asset, Amount: in},
	})
	if err != nil {
		l.t.Fatalf("testkit.Ledger.WithHolding: %v", err)
	}
	l.do(func(ctx context.Context, tx db.Tx) error { return l.ledger.PostCabalTxn(ctx, tx, txn) })
	return l
}

func (l *Ledger) signed(v uint64) (money.SignedMicros, money.SignedMicros) {
	l.t.Helper()
	raw := strconv.FormatUint(v, 10)
	in, err := money.ParseSignedMicros(raw)
	out, err2 := money.ParseSignedMicros("-" + raw)
	if err != nil || err2 != nil || v == 0 {
		l.t.Fatalf("testkit.Ledger: amount %d is not a positive int64", v)
	}
	return in, out
}

func (l *Ledger) do(fn func(ctx context.Context, tx db.Tx) error) {
	l.t.Helper()
	if err := l.uow.Do(l.t.Context(), fn); err != nil {
		l.t.Fatalf("testkit.Ledger: %v", err)
	}
}
