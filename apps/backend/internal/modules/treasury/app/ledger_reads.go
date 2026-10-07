package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type LedgerReads struct {
	q *sqlc.Queries
}

var _ port.LedgerReader = (*LedgerReads)(nil)

func NewLedgerReads(db sqlc.DBTX) *LedgerReads { return &LedgerReads{q: sqlc.New(db)} }

func (r *LedgerReads) UserTxns(
	ctx context.Context, user ids.UserID, cursor *port.TxnCursor, limit int,
) ([]port.TxnHeader, error) {
	const op = "treasury.LedgerReads.UserTxns"
	rows, err := r.q.AdminUserTxns(ctx, sqlc.AdminUserTxnsParams{
		UserID: user.UUID(), HasCursor: cursor != nil, CursorAt: cursorAt(cursor), CursorID: cursorID(cursor),
		RowLimit: int64(limit),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, op)
	}
	out := make([]port.TxnHeader, len(rows))
	for i, row := range rows {
		userID := ids.UserIDFrom(row.UserID)
		out[i] = port.TxnHeader{
			ID:          row.ID,
			Scope:       port.TxnScopeUser,
			UserID:      &userID,
			CabalID:     optionalCabal(row.CabalID.Valid, row.CabalID.Bytes),
			Kind:        row.Kind,
			Status:      row.Status,
			TxSignature: chain.Signature(row.TxSignature.String),
			CreatedAt:   row.CreatedAt.UTC(),
		}
	}
	return out, nil
}

func (r *LedgerReads) CabalTxns(
	ctx context.Context, cabal ids.CabalID, cursor *port.TxnCursor, limit int,
) ([]port.TxnHeader, error) {
	const op = "treasury.LedgerReads.CabalTxns"
	rows, err := r.q.AdminCabalTxns(ctx, sqlc.AdminCabalTxnsParams{
		CabalID: cabal.UUID(), HasCursor: cursor != nil, CursorAt: cursorAt(cursor), CursorID: cursorID(cursor),
		RowLimit: int64(limit),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, op)
	}
	out := make([]port.TxnHeader, len(rows))
	for i, row := range rows {
		cabalID := ids.CabalIDFrom(row.CabalID)
		out[i] = port.TxnHeader{
			ID: row.ID, Scope: port.TxnScopeCabal, CabalID: &cabalID, Kind: row.Kind, Status: row.Status,
			TxSignature: chain.Signature(row.TxSignature.String), CreatedAt: row.CreatedAt.UTC(),
		}
	}
	return out, nil
}

func (r *LedgerReads) TxnByID(ctx context.Context, id uuid.UUID) (port.Txn, bool, error) {
	const op = "treasury.LedgerReads.TxnByID"
	rows, err := r.q.AdminTxnByID(ctx, id)
	if err != nil {
		return port.Txn{}, false, errs.Wrap(err, errs.CodeDBUnavailable, op)
	}
	return assembleTxn(op, rows)
}

func (r *LedgerReads) TxnBySignature(ctx context.Context, sig chain.Signature) (port.Txn, bool, error) {
	const op = "treasury.LedgerReads.TxnBySignature"
	rows, err := r.q.AdminTxnBySignature(ctx, string(sig))
	if err != nil {
		return port.Txn{}, false, errs.Wrap(err, errs.CodeDBUnavailable, op)
	}
	flat := make([]txnRow, len(rows))
	for i, row := range rows {
		flat[i] = txnRow(row)
	}
	return assembleTxn(op, flat)
}

func (r *LedgerReads) UserShares(ctx context.Context, user ids.UserID) ([]port.Share, error) {
	const op = "treasury.LedgerReads.UserShares"
	rows, err := r.q.AdminUserShares(ctx, user.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, op)
	}
	flat := make([]shareRow, len(rows))
	copy(flat, rows)
	return shares(op, flat)
}

func (r *LedgerReads) CabalShares(ctx context.Context, cabal ids.CabalID) ([]port.Share, error) {
	const op = "treasury.LedgerReads.CabalShares"
	rows, err := r.q.AdminCabalShares(ctx, cabal.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, op)
	}
	flat := make([]shareRow, len(rows))
	for i, row := range rows {
		flat[i] = shareRow(row)
	}
	return shares(op, flat)
}

func (r *LedgerReads) CabalHoldings(ctx context.Context, cabal ids.CabalID) ([]port.RawHolding, error) {
	const op = "treasury.LedgerReads.CabalHoldings"
	rows, err := r.q.AdminCabalHoldings(ctx, cabal.UUID())
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, op)
	}
	out := make([]port.RawHolding, 0, len(rows))
	for _, row := range rows {
		units, err := money.ParseMicros(row.Units)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("value", row.Units))
		}
		out = append(out, port.RawHolding{
			Mint: chain.SolanaAddress(row.Asset), Units: money.NewBaseUnits(units.Uint64(), 0),
		})
	}
	return out, nil
}

type shareRow = sqlc.AdminUserSharesRow

func shares(op string, rows []shareRow) ([]port.Share, error) {
	out := make([]port.Share, 0, len(rows))
	for _, row := range rows {
		units, err := money.ParseSharesUnits(row.ShareUnits)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("value", row.ShareUnits))
		}
		out = append(out, port.Share{
			CabalID: ids.CabalIDFrom(row.CabalID), UserID: ids.UserIDFrom(row.UserID), ShareUnits: units,
		})
	}
	return out, nil
}

type txnRow = sqlc.AdminTxnByIDRow

func assembleTxn(op string, rows []txnRow) (port.Txn, bool, error) {
	if len(rows) == 0 {
		return port.Txn{}, false, nil
	}
	first := rows[0]
	txn := port.Txn{TxnHeader: port.TxnHeader{
		ID:      first.ID,
		Scope:   port.TxnScope(first.Scope),
		CabalID: optionalCabal(first.CabalID.Valid, first.CabalID.Bytes),
		Kind:    first.Kind,
		Status:  first.Status,
		SwapID:  optionalSwap(first.SwapID.Valid, first.SwapID.Bytes),
		TransferID: optionalUUID(
			first.TransferID.Valid,
			first.TransferID.Bytes,
		),
		TxSignature: chain.Signature(first.TxSignature.String),
		CreatedAt:   first.CreatedAt.UTC(),
	}}
	if first.UserID != "" {
		user, err := uuid.Parse(first.UserID)
		if err != nil {
			return port.Txn{}, false, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("user_id", first.UserID))
		}
		userID := ids.UserIDFrom(user)
		txn.UserID = &userID
	}
	for _, row := range rows {
		if row.HasEntry {
			txn.Entries = append(txn.Entries, port.TxnEntry{
				Seq: row.Seq, Account: row.Account, Asset: row.Asset, Amount: row.Amount,
			})
		}
	}
	return txn, true, nil
}

func cursorAt(c *port.TxnCursor) (at time.Time) {
	if c != nil {
		at = c.At
	}
	return at
}

func cursorID(c *port.TxnCursor) (id uuid.UUID) {
	if c != nil {
		id = c.ID
	}
	return id
}

func optionalCabal(valid bool, b [16]byte) *ids.CabalID {
	if !valid {
		return nil
	}
	id := ids.CabalIDFrom(b)
	return &id
}

func optionalSwap(valid bool, b [16]byte) *ids.SwapID {
	if !valid {
		return nil
	}
	id := ids.SwapIDFrom(b)
	return &id
}

func optionalUUID(valid bool, b [16]byte) *uuid.UUID {
	if !valid {
		return nil
	}
	id := uuid.UUID(b)
	return &id
}
