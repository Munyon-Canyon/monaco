package app

import (
	"context"
	"math"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const depositCandidateSourceRecovery = "recovery"

type DepositWatchTuning struct {
	Rotation      time.Duration
	RecoverySlots int64
	Discovery     time.Duration
	Spread        func(period time.Duration) time.Duration
}

func cutBelow(page []solana.SignatureInfo, floor int64) ([]solana.SignatureInfo, bool) {
	i := slices.IndexFunc(page, func(sig solana.SignatureInfo) bool {
		return int64(min(sig.Slot, math.MaxInt64)) < floor
	})
	if i < 0 {
		return page, false
	}
	return page[:i], true
}

func (p *DepositWatch) rotate(ctx context.Context) (int, int, error) {
	rows, err := sqlc.New(p.reads).DepositWatchRotationAccounts(ctx, sqlc.DepositWatchRotationAccountsParams{
		Now: p.clock.Now(), RowLimit: depositWatchDirtyBatch,
	})
	if err != nil {
		return 0, 0, errs.Wrap(err, errs.CodeOf(err), "funding.DepositWatch.rotate")
	}
	var scanned, recorded int
	for _, row := range rows {
		n, err := p.recoverAccount(ctx, row)
		recorded += n
		if err != nil {
			return scanned, recorded, err
		}
		scanned++
	}
	return scanned, recorded, nil
}

func (p *DepositWatch) recoverAccount(ctx context.Context, row sqlc.DepositWatchRotationAccountsRow) (int, error) {
	floor := max(row.HighSlot-p.tuning.RecoverySlots, row.FirstSeenSlot)
	opts := solana.SignaturesOpts{Before: chain.Signature(row.RecoveryBefore.String), Limit: depositSignaturePageSize}
	var recorded int
	for {
		page, err := p.signatures(ctx, chain.SolanaAddress(row.TokenAccount), opts)
		if err != nil {
			return recorded, err
		}
		more := len(page) == depositSignaturePageSize
		page, cut := cutBelow(page, floor)
		more = more && !cut
		n, err := p.commitRecoveryPage(ctx, row, page, more)
		recorded += n
		if err != nil || !more {
			return recorded, err
		}
		opts.Before = page[len(page)-1].Signature
	}
}

func (p *DepositWatch) commitRecoveryPage(
	ctx context.Context, row sqlc.DepositWatchRotationAccountsRow, page []solana.SignatureInfo, more bool,
) (int, error) {
	candidates, err := candidatesFor(
		row.WalletAddress, ids.UserIDFrom(row.UserID), depositCandidateSourceRecovery, page)
	if err != nil {
		return 0, err
	}
	var recorded int
	err = p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		if recorded, err = NewCandidateRecorder(p.ids, p.clock.Now).Record(ctx, tx, candidates); err != nil {
			return err
		}
		q := sqlc.New(tx.Queries())
		if more {
			return q.CheckpointDepositWatchRecovery(ctx, sqlc.CheckpointDepositWatchRecoveryParams{
				TokenAccount: row.TokenAccount, RecoveryBefore: string(page[len(page)-1].Signature),
				ScannedAt: p.clock.Now(),
			})
		}
		return q.CompleteDepositWatchRecovery(ctx, sqlc.CompleteDepositWatchRecoveryParams{
			TokenAccount:  row.TokenAccount,
			RecoveryDueAt: p.clock.Now().Add(p.tuning.Rotation),
			ScannedAt:     p.clock.Now(),
		})
	})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), "funding.DepositWatch.commitRecoveryPage")
	}
	return recorded, nil
}

func (p *DepositWatch) discover(ctx context.Context) (int, int, error) {
	rows, err := sqlc.New(p.reads).DepositWatchDiscoveryWallets(ctx, sqlc.DepositWatchDiscoveryWalletsParams{
		Now: p.clock.Now(), RowLimit: depositWatchDirtyBatch,
	})
	if err != nil {
		return 0, 0, errs.Wrap(err, errs.CodeOf(err), "funding.DepositWatch.discover")
	}
	var scanned int
	for _, row := range rows {
		if err := p.discoverWallet(ctx, row.WalletAddress); err != nil {
			return scanned, 0, err
		}
		scanned++
	}
	return scanned, 0, nil
}

func (p *DepositWatch) discoverWallet(ctx context.Context, wallet string) error {
	if err := p.wait(ctx); err != nil {
		return err
	}
	slot, accounts, err := p.rpc.TokenAccounts(
		ctx, chain.SolanaAddress(wallet), chain.Mint{Address: p.usdc, Decimals: usdcDecimals},
	)
	if err != nil {
		return errs.Wrap(err, errs.CodeRPCUnavailable, "funding.DepositWatch.discover")
	}
	if slot > math.MaxInt64 {
		return errs.New(errs.CodeInternal, "funding.DepositWatch.discover")
	}
	observed := int64(slot)
	err = p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		for _, account := range accounts {
			if err := q.InsertDepositWatchAccount(ctx, sqlc.InsertDepositWatchAccountParams{
				TokenAccount:  string(account.Address),
				WalletAddress: wallet,
				State:         watchStateOpen,
				LastAmount:    account.Amount.String(),
				ObservedSlot:  observed,
				DirtyGen:      1,
				DirtySlot:     observed,
				RecoveryDueAt: p.clock.Now().Add(p.tuning.Spread(p.tuning.Rotation)),
			}); err != nil {
				return err
			}
		}
		return q.SetDepositWatchDiscovery(ctx, sqlc.SetDepositWatchDiscoveryParams{
			WalletAddress: wallet, DiscoveryDueAt: p.clock.Now().Add(p.tuning.Discovery),
		})
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeOf(err), "funding.DepositWatch.discoverWallet")
	}
	return nil
}
