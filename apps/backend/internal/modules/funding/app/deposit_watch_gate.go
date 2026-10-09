package app

import (
	"context"
	"math"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

const (
	WatchRegressedCounter = "monaco_funding_watch_regressed_total"

	depositWatchGatePage = 100

	watchStateOpen    = "open"
	watchStateClosed  = "closed"
	watchStateForeign = "foreign"

	tokenAccountInitialized = "initialized"
)

func (p *DepositWatch) gate(ctx context.Context) (int, int, error) {
	var after string
	var scanned int
	for {
		rows, err := sqlc.New(p.reads).DepositWatchGateAccounts(ctx, sqlc.DepositWatchGateAccountsParams{
			TokenAccount: after, Limit: depositWatchGatePage,
		})
		if err != nil {
			return scanned, 0, errs.Wrap(err, errs.CodeOf(err), "funding.DepositWatch.gate")
		}
		if len(rows) == 0 {
			return scanned, 0, nil
		}
		if err := p.gatePage(ctx, rows); err != nil {
			return scanned, 0, err
		}
		scanned += len(rows)
		if len(rows) < depositWatchGatePage {
			return scanned, 0, nil
		}
		after = rows[len(rows)-1].TokenAccount
	}
}

func (p *DepositWatch) gatePage(ctx context.Context, rows []sqlc.DepositWatchGateAccountsRow) error {
	addresses := make([]chain.SolanaAddress, len(rows))
	for i, row := range rows {
		addresses[i] = chain.SolanaAddress(row.TokenAccount)
	}
	if err := p.wait(ctx); err != nil {
		return err
	}
	slot, observed, err := p.rpc.Accounts(ctx, addresses, 0)
	if err != nil {
		return errs.Wrap(err, errs.CodeRPCUnavailable, "funding.DepositWatch.accounts")
	}
	if len(observed) != len(rows) {
		return errs.New(errs.CodeDecodeFailed, "funding.DepositWatch.accounts")
	}
	updates := make([]sqlc.ApplyDepositWatchObservationParams, 0, len(rows))
	for i, row := range rows {
		if int64(min(slot, math.MaxInt64)) < row.ObservedSlot {
			p.regressed.Add(ctx, 1)
			continue
		}
		if update, ok := p.observation(row, observed[i], slot); ok {
			updates = append(updates, update)
		}
	}
	err = p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		for _, update := range updates {
			if _, err := q.ApplyDepositWatchObservation(ctx, update); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeOf(err), "funding.DepositWatch.gatePage")
	}
	return nil
}

func (p *DepositWatch) observation(
	row sqlc.DepositWatchGateAccountsRow, got solana.TokenAccountState, slot uint64,
) (sqlc.ApplyDepositWatchObservationParams, bool) {
	update := sqlc.ApplyDepositWatchObservationParams{
		TokenAccount: row.TokenAccount,
		State:        row.State,
		LastAmount:   "0",
		ObservedSlot: int64(min(slot, math.MaxInt64)),
	}
	wasOpen := row.State == watchStateOpen
	switch {
	case !got.Exists:
		update.Dirty = wasOpen
		if wasOpen {
			update.State = watchStateClosed
		}
	case got.Owner != chain.SolanaAddress(row.WalletAddress):
		update.State, update.Dirty, update.LastAmount = watchStateForeign, true, got.Amount.String()
	case got.Program != chain.SPLProgram || got.Mint != p.usdc || got.State != tokenAccountInitialized:
		return update, false
	default:
		update.State, update.LastAmount = watchStateOpen, got.Amount.String()
		update.Dirty = !wasOpen || row.LastAmount != update.LastAmount
	}
	return update, true
}
