package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const sharePriceUnit = 1_000_000

type fundMint struct {
	transfer openFund
	user     ids.UserID
	cabal    ids.CabalID
	amount   money.Micros
}

func (s *FundSettler) mint(ctx context.Context, r openFund) (bool, error) {
	amount, err := money.ParseMicros(r.AmountMicros)
	if err != nil {
		return false, err
	}
	m := fundMint{transfer: r, user: ids.UserIDFrom(r.UserID), cabal: ids.CabalIDFrom(r.CabalID), amount: amount}
	var funded events.Funded
	var settled bool
	err = s.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		funded, settled, err = s.settle(ctx, tx, m)
		return err
	})
	switch {
	case err == nil && !settled:
		return false, nil
	case errs.CodeOf(err) == errs.CodePriceUnavailable:
		observability.Info(ctx, observability.TreasuryFundMintWaiting, slog.String("transfer_id", r.ID.String()),
			slog.String("cabal_id", m.cabal.String()))
		return false, nil
	case err != nil:
		return false, err
	}
	observability.Info(
		ctx,
		observability.TreasuryFundSettled,
		slog.String("transfer_id", r.ID.String()),
		slog.String("cabal_id", m.cabal.String()),
		slog.String("share_units", funded.ShareUnits.String()),
		slog.String(
			"status_before",
			string(domain.FundLanded),
		),
		slog.String("status_after", string(domain.FundSettled)),
	)
	return true, nil
}

func (s *FundSettler) settle(ctx context.Context, tx db.Tx, m fundMint) (events.Funded, bool, error) {
	lockErr := s.d.Ledger.LockCabal(ctx, tx, m.cabal)
	pot, potErr := s.d.Pot.PotValue(ctx, m.cabal)
	total, totalErr := s.d.Pot.TotalShares(ctx, m.cabal)
	if err := errors.Join(lockErr, potErr, totalErr); err != nil {
		return events.Funded{}, false, err
	}
	units, err := domain.MintShares(m.amount, total, pot)
	if err != nil {
		return events.Funded{}, false, err
	}
	funded, err := fundedEvent(m, units, total, pot)
	if err != nil {
		return events.Funded{}, false, err
	}
	n, err := sqlc.New(tx.Queries()).SettleFundTransfer(ctx, sqlc.SettleFundTransferParams{
		ID: m.transfer.ID, ShareUnits: units.String(), SettledAt: s.d.Clock.Now(),
	})
	if err != nil {
		return events.Funded{}, false, err
	}
	if n == 0 {
		return events.Funded{}, false, nil
	}
	if err := s.post(ctx, tx, m, units); err != nil {
		return events.Funded{}, false, err
	}
	tx.AfterCommit(func(ctx context.Context) {
		s.d.Hints.PublishHint(ctx, events.UserBalanceChangedHint(m.user), nil)
		s.d.Hints.PublishHint(ctx, "cabal."+m.cabal.String()+".activity_changed", nil)
	})
	return funded, true, tx.Events.Append(ctx, funded)
}

func (s *FundSettler) post(ctx context.Context, tx db.Tx, m fundMint, units money.SharesUnits) error {
	usdc, shares, sig := s.d.USDC, domain.SharesAsset(m.cabal), chain.Signature(m.transfer.TxSignature)
	in, minted, err := ledgerAmounts(m.amount, units)
	u, userErr := domain.NewUserTxn(domain.UserTxnHeader{
		ID: s.d.IDs.NewV7(), UserID: m.user, CabalID: m.cabal, Kind: domain.UserFund, Status: domain.TxnSettled,
		TransferID: m.transfer.ID, TxSignature: sig,
	}, []domain.UserEntry{
		{Account: domain.UserWallet, Asset: usdc, Amount: neg(in)},
		{Account: domain.UserCabal, Asset: usdc, Amount: in},
		{Account: domain.UserHolder, Asset: shares, Amount: minted},
		{Account: domain.UserIssuer, Asset: shares, Amount: neg(minted)},
	})
	c, cabalErr := domain.NewCabalTxn(domain.CabalTxnHeader{
		ID: s.d.IDs.NewV7(), CabalID: m.cabal, Kind: domain.CabalFund, Status: domain.TxnSettled,
		TransferID: m.transfer.ID, TxSignature: sig,
	}, []domain.CabalEntry{
		{Account: domain.CabalTreasury, Asset: usdc, Amount: in},
		{Account: domain.CabalMembers, Asset: usdc, Amount: neg(in)},
	})
	if err := errors.Join(err, userErr, cabalErr); err != nil {
		return err
	}
	if err := s.d.Ledger.PostUserTxn(ctx, tx, u); err != nil {
		return err
	}
	return s.d.Ledger.PostCabalTxn(ctx, tx, c)
}

func ledgerAmounts(amount money.Micros, units money.SharesUnits) (in, minted money.SignedMicros, err error) {
	in, inErr := amount.Delta(money.Micros{})
	minted, mintErr := money.MicrosFromUint64(units.Uint64()).Delta(money.Micros{})
	return in, minted, errors.Join(inErr, mintErr)
}

func neg(v money.SignedMicros) money.SignedMicros { return money.SignedMicrosFromInt64(-v.Int64()) }

func fundedEvent(m fundMint, units, total money.SharesUnits, pot money.Micros) (events.Funded, error) {
	after, addErr := total.Add(units)
	price, priceErr := sharePrice(total, pot)
	return events.Funded{
		V: 1, TransferID: m.transfer.ID, CabalID: m.cabal.UUID(), UserID: m.user.UUID(), AmountMicros: m.amount,
		ShareUnits: units, SharePriceMicros: price, PotValueBeforeMicros: pot, TotalSharesAfter: after,
		TxSignature: chain.Signature(m.transfer.TxSignature),
	}, errors.Join(addErr, priceErr)
}

func sharePrice(total money.SharesUnits, pot money.Micros) (money.Micros, error) {
	if total.IsZero() {
		return money.MicrosFromUint64(sharePriceUnit), nil
	}
	raw, err := money.MulDiv(pot.Uint64(), sharePriceUnit, total.Uint64())
	return money.MicrosFromUint64(raw), err
}
