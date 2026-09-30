package app

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type Ledger struct {
	usdc  domain.Asset
	clock clock.Clock
}

func NewLedger(usdc chain.SolanaAddress, c clock.Clock) Ledger {
	return Ledger{usdc: domain.MintAsset(usdc), clock: c}
}

func (Ledger) LockCabal(ctx context.Context, tx db.Tx, cabal ids.CabalID) error {
	return sqlc.New(tx.Queries()).LockCabalLedger(ctx, cabal.UUID())
}

func (l Ledger) PostCabalTxn(ctx context.Context, tx db.Tx, t domain.CabalTxn) error {
	const op = "treasury.Ledger.PostCabalTxn"
	deltas, err := t.PositionDeltas(l.usdc)
	if err != nil {
		return err
	}
	if err := l.LockCabal(ctx, tx, t.CabalID); err != nil {
		return err
	}
	q := sqlc.New(tx.Queries())
	n, err := q.InsertCabalTxn(ctx, sqlc.InsertCabalTxnParams{
		ID: t.ID, CabalID: t.CabalID.UUID(), Kind: string(t.Kind), Status: string(t.Status), SwapID: t.SwapID,
		TransferID: t.TransferID, TxSignature: string(t.TxSignature), CreatedAt: l.clock.Now(),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return statusSplit(op, t.TransferID, t.Status)
	}
	var seq int16
	for _, e := range t.Entries() {
		if err := q.InsertCabalEntry(ctx, sqlc.InsertCabalEntryParams{
			TxnID: t.ID, Seq: seq, Account: string(e.Account), Asset: string(e.Asset), Amount: e.Amount.Int64(),
		}); err != nil {
			return err
		}
		seq++
	}
	posted := make([]posting, 0, len(deltas))
	for _, d := range deltas {
		row, err := q.ApplyCabalPosition(ctx, sqlc.ApplyCabalPositionParams{
			CabalID: t.CabalID.UUID(), Asset: string(d.Asset), Delta: d.Units.Int64(), CostIn: d.CostIn.String(),
			UpdatedAt: l.clock.Now(),
		})
		if err != nil {
			return err
		}
		posted = append(posted, posting{t.CabalID, d.Asset, row.Before, row.After, d.Units.String()})
	}
	logCabalPosted(tx, posted)
	return nil
}

func (l Ledger) PostUserTxn(ctx context.Context, tx db.Tx, t domain.UserTxn) error {
	const op = "treasury.Ledger.PostUserTxn"
	delta, err := t.PositionDelta()
	if err != nil {
		return err
	}
	if !t.CabalID.IsZero() {
		if err := l.LockCabal(ctx, tx, t.CabalID); err != nil {
			return err
		}
	}
	q := sqlc.New(tx.Queries())
	n, err := q.InsertUserTxn(ctx, sqlc.InsertUserTxnParams{
		ID: t.ID, UserID: t.UserID.UUID(), CabalID: t.CabalID.UUID(), Kind: string(t.Kind), Status: string(t.Status),
		TransferID: t.TransferID, TxSignature: string(t.TxSignature), CreatedAt: l.clock.Now(),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return statusSplit(op, t.TransferID, t.Status)
	}
	var seq int16
	for _, e := range t.Entries() {
		if err := q.InsertUserEntry(ctx, sqlc.InsertUserEntryParams{
			TxnID: t.ID, Seq: seq, Account: string(e.Account), Asset: string(e.Asset), Amount: e.Amount.Int64(),
		}); err != nil {
			return err
		}
		seq++
	}
	if t.CabalID.IsZero() {
		return nil
	}
	row, err := q.ApplyUserPosition(ctx, sqlc.ApplyUserPositionParams{
		UserID: t.UserID.UUID(), CabalID: t.CabalID.UUID(), Shares: delta.Shares.Int64(),
		Contributed: delta.Contributed.String(), Withdrawn: delta.Withdrawn.String(), UpdatedAt: l.clock.Now(),
	})
	if err != nil {
		return err
	}
	logUserPosted(tx, t.UserID,
		posting{t.CabalID, domain.SharesAsset(t.CabalID), row.Before, row.After, delta.Shares.String()})
	return nil
}

func (Ledger) SetStatus(ctx context.Context, tx db.Tx, transferID uuid.UUID, from, to domain.TxnStatus) (bool, error) {
	const op = "treasury.Ledger.SetStatus"
	if !from.CanMoveTo(to) {
		return false, errs.New(errs.CodeInvalidInput, op,
			slog.String("from", string(from)), slog.String("to", string(to)))
	}
	row, err := sqlc.New(tx.Queries()).SetTransferStatus(ctx, sqlc.SetTransferStatusParams{
		ToStatus: string(to), TransferID: transferID, FromStatus: string(from),
	})
	if err != nil {
		return false, err
	}
	if row.CabalRows != row.UserRows {
		return false, errs.New(errs.CodeInternal, op, slog.String("transfer_id", transferID.String()),
			slog.Int64("cabal_rows", row.CabalRows), slog.Int64("user_rows", row.UserRows))
	}
	return row.CabalRows > 0, nil
}

func statusSplit(op string, transferID uuid.UUID, status domain.TxnStatus) error {
	return errs.New(errs.CodeInternal, op,
		slog.String("transfer_id", transferID.String()), slog.String("status", string(status)))
}

type posting struct {
	cabal  ids.CabalID
	asset  domain.Asset
	before string
	after  string
	delta  string
}

func logCabalPosted(tx db.Tx, posted []posting) {
	tx.AfterCommit(func(ctx context.Context) {
		for _, p := range posted {
			observability.Info(ctx, observability.TreasuryLedgerPosted,
				slog.String("cabal_id", p.cabal.String()), slog.String("asset", string(p.asset)),
				slog.String("before", p.before), slog.String("after", p.after), slog.String("delta", p.delta))
		}
	})
}

func logUserPosted(tx db.Tx, user ids.UserID, p posting) {
	tx.AfterCommit(func(ctx context.Context) {
		observability.Info(ctx, observability.TreasuryLedgerPosted,
			slog.String("cabal_id", p.cabal.String()), slog.String("asset", string(p.asset)),
			slog.String("before", p.before), slog.String("after", p.after), slog.String("delta", p.delta),
			slog.String("user_id", user.String()))
	})
}
