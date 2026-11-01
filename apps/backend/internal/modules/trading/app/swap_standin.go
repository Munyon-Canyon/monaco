package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const ParkedSwapSignatureUnknown = "swap_signature_unknown"

func (p *SwapSweeper) recoverStandIn(
	ctx context.Context, now time.Time, row sqlc.ListStaleSubmittedRow,
) (chain.Signature, error) {
	standIn := chain.Signature(row.TxSignature.String)
	signature, found, err := p.chain.FindSignature(ctx, chain.SolanaAddress(row.TreasuryAddress), standIn)
	if err != nil {
		return "", err
	}
	if found {
		return signature, p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
			_, err := sqlc.New(tx.Queries()).AdoptSignature(ctx, sqlc.AdoptSignatureParams{
				ID: row.ID, StandIn: string(standIn), TxSignature: string(signature), At: now,
			})
			return err
		})
	}
	valid, err := p.chain.BlockhashValid(ctx, row.SignedTx)
	if err != nil || valid {
		return "", err
	}
	return "", p.parkUnknownSignature(ctx, now, row.ID)
}

func (p *SwapSweeper) parkUnknownSignature(ctx context.Context, now time.Time, id uuid.UUID) error {
	return p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		row, err := sqlc.New(tx.Queries()).ParkRouted(ctx, sqlc.ParkRoutedParams{
			At: now, Code: ParkedSwapSignatureUnknown, ID: id,
		})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) {
			observability.Alert(ctx, observability.TradingRoutedSwapParked,
				slog.String("swap_id", id.String()), slog.String("cabal_id", row.CabalID.String()),
				slog.String("code", ParkedSwapSignatureUnknown), slog.String("step", row.Step))
		})
		return nil
	})
}
