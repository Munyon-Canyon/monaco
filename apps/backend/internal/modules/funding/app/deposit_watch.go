package app

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	identity "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const depositWatchStopMargin = 5 * time.Second

type DepositWatchRPC interface {
	SignaturesFor(context.Context, chain.SolanaAddress, solana.SignaturesOpts) ([]solana.SignatureInfo, error)
	TokenAccounts(context.Context, chain.SolanaAddress, chain.Mint) (uint64, []solana.TokenAccountState, error)
}

type DepositWatch struct {
	reads   sqlc.DBTX
	uow     *db.UnitOfWork
	ids     ids.Generator
	clock   clock.Clock
	wallets identity.WalletReader
	rpc     DepositWatchRPC
	usdc    chain.SolanaAddress
	period  time.Duration
	budget  int32
	limit   RPCLimiter
}

type watchSeed struct {
	state    solana.TokenAccountState
	high     chain.Signature
	highSlot uint64
}

func NewDepositWatch(
	reads sqlc.DBTX,
	uow *db.UnitOfWork,
	g ids.Generator,
	c clock.Clock,
	wallets identity.WalletReader,
	rpc DepositWatchRPC,
	usdc chain.SolanaAddress,
	period time.Duration,
	budget int32,
	limit RPCLimiter,
) *DepositWatch {
	budget = max(budget, 480)
	return &DepositWatch{
		reads: reads, uow: uow, ids: g, clock: c, wallets: wallets, rpc: rpc, usdc: usdc,
		period: period, budget: budget, limit: limit,
	}
}

func (*DepositWatch) Name() string { return "funding.deposit_watch" }

func (p *DepositWatch) Interval() time.Duration { return p.period }

func (p *DepositWatch) Tick(ctx context.Context) (poller.Report, error) {
	var report poller.Report
	deadline := p.clock.Now().Add(p.period - depositWatchStopMargin)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d.Add(-depositWatchStopMargin)
	}
	calls := int32(0)
	dirty, err := p.catchUpDirty(ctx, deadline, &calls)
	if err != nil {
		return report, err
	}
	newWallets, err := p.firstSight(ctx, deadline, &calls)
	if err != nil {
		return report, err
	}
	report.Scanned = dirty + newWallets
	report.Attrs = []slog.Attr{
		slog.Int("dirty", dirty),
		slog.Int("first_sight", newWallets),
		slog.Int("calls", int(calls)),
	}
	return report, nil
}

func (p *DepositWatch) available(deadline time.Time, calls *int32) bool {
	return *calls < p.budget && p.clock.Now().Before(deadline)
}

func (p *DepositWatch) catchUpDirty(ctx context.Context, deadline time.Time, calls *int32) (int, error) {
	rows, err := sqlc.New(p.reads).DepositWatchDirtyAccounts(ctx, p.budget)
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), "funding.DepositWatch.dirty")
	}
	var scanned int
	for _, row := range rows {
		if !p.available(deadline, calls) {
			break
		}
		if err := p.catchUpAccount(ctx, row, calls); err != nil {
			return scanned, err
		}
		scanned++
	}
	return scanned, nil
}

func (p *DepositWatch) catchUpAccount(ctx context.Context, row sqlc.DepositWatchDirtyAccountsRow, calls *int32) error {
	minSlot, err := watchMinSlot(row)
	if err != nil {
		return err
	}
	page, err := p.signatures(ctx, chain.SolanaAddress(row.TokenAccount), chain.Signature(row.PageBefore.String),
		chain.Signature(row.HighSignature.String), minSlot, calls)
	if err != nil {
		return err
	}
	return p.uow.Do(
		observability.WithActor(ctx, "system:funding.deposit_watch"),
		func(ctx context.Context, tx db.Tx) error {
			return p.commitCatchUp(ctx, tx, row, page)
		},
	)
}

func watchMinSlot(row sqlc.DepositWatchDirtyAccountsRow) (uint64, error) {
	if row.DirtySlot < 0 || row.ObservedSlot < 0 || row.HighSlot < 0 {
		return 0, errs.New(errs.CodeInternal, "funding.DepositWatch.catchUp")
	}
	return uint64(max(row.DirtySlot, row.ObservedSlot)), nil
}

func (p *DepositWatch) commitCatchUp(
	ctx context.Context,
	tx db.Tx,
	row sqlc.DepositWatchDirtyAccountsRow,
	page []solana.SignatureInfo,
) error {
	if _, err := NewCandidateRecorder(p.ids, p.clock.Now).Record(ctx, tx, watchCandidates(row, page)); err != nil {
		return err
	}
	q := sqlc.New(tx.Queries())
	if len(page) == 0 {
		return q.CompleteDepositWatchPage(
			ctx,
			sqlc.CompleteDepositWatchPageParams{
				TokenAccount: row.TokenAccount,
				CleanGen:     row.DirtyGen,
				ScannedAt:    p.clock.Now(),
			},
		)
	}
	first, last := page[0], page[len(page)-1]
	if first.Slot > math.MaxInt64 || last.Slot > math.MaxInt64 {
		return errs.New(errs.CodeInternal, "funding.DepositWatch.catchUp")
	}
	params := sqlc.CheckpointDepositWatchPageParams{
		TokenAccount:     row.TokenAccount,
		PageBefore:       string(last.Signature),
		PageTopSignature: string(first.Signature),
		PageTopSlot:      int64(first.Slot),
		ScannedAt:        p.clock.Now(),
	}
	if err := q.CheckpointDepositWatchPage(ctx, params); err != nil {
		return err
	}
	if len(page) < depositSignaturePageSize {
		return q.CompleteDepositWatchPage(
			ctx,
			sqlc.CompleteDepositWatchPageParams{
				TokenAccount: row.TokenAccount,
				CleanGen:     row.DirtyGen,
				ScannedAt:    p.clock.Now(),
			},
		)
	}
	return nil
}

func watchCandidates(row sqlc.DepositWatchDirtyAccountsRow, page []solana.SignatureInfo) []DepositCandidate {
	candidates := make([]DepositCandidate, 0, len(page))
	for _, sig := range page {
		if !sig.Failed && sig.Slot <= math.MaxInt64 {
			candidates = append(
				candidates,
				DepositCandidate{
					Signature: sig.Signature,
					Wallet:    chain.SolanaAddress(row.WalletAddress),
					UserID:    ids.UserIDFrom(row.UserID),
					Slot:      int64(sig.Slot),
					BlockTime: sig.BlockTime,
					Source:    "poller",
				},
			)
		}
	}
	return candidates
}

func (p *DepositWatch) firstSight(ctx context.Context, deadline time.Time, calls *int32) (int, error) {
	if !p.available(deadline, calls) {
		return 0, nil
	}
	page, err := p.wallets.MemberWallets(ctx, ids.UserID{}, identity.MaxWalletPage)
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), "funding.DepositWatch.firstSight")
	}
	addresses := make([]string, len(page))
	for i, wallet := range page {
		addresses[i] = string(wallet.Address)
	}
	knownRows, err := sqlc.New(p.reads).DepositWatchKnownWallets(ctx, addresses)
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), "funding.DepositWatch.firstSight")
	}
	known := make(map[string]struct{}, len(knownRows))
	for _, row := range knownRows {
		known[row] = struct{}{}
	}
	var added int
	for _, wallet := range page {
		if _, ok := known[string(wallet.Address)]; ok || !p.available(deadline, calls) {
			continue
		}
		if err := p.seedWallet(ctx, wallet, calls); err != nil {
			return added, err
		}
		added++
	}
	return added, nil
}

func (p *DepositWatch) seedWallet(ctx context.Context, wallet identity.MemberWallet, calls *int32) error {
	if err := p.wait(ctx); err != nil {
		return err
	}
	*calls++
	slot, accounts, err := p.rpc.TokenAccounts(ctx, wallet.Address, chain.Mint{Address: p.usdc, Decimals: 6})
	if err != nil {
		return errs.Wrap(err, errs.CodeRPCUnavailable, "funding.DepositWatch.seed")
	}
	canonical, err := chain.AssociatedTokenAccount(wallet.Address, p.usdc, chain.SPLProgram)
	if err != nil {
		return errs.Wrap(err, errs.CodeInvalidAddress, "funding.DepositWatch.seed")
	}
	seeds, err := p.watchSeeds(ctx, canonical, accounts, slot, calls)
	if err != nil {
		return err
	}
	slotInt, err := intSlot(slot)
	if err != nil {
		return err
	}
	return p.persistSeeds(ctx, wallet, canonical, seeds, slotInt)
}

func (p *DepositWatch) watchSeeds(
	ctx context.Context,
	canonical chain.SolanaAddress,
	accounts []solana.TokenAccountState,
	slot uint64,
	calls *int32,
) ([]watchSeed, error) {
	seen := map[chain.SolanaAddress]solana.TokenAccountState{canonical: {Address: canonical}}
	for _, account := range accounts {
		seen[account.Address] = account
	}
	seeds := make([]watchSeed, 0, len(seen))
	for _, account := range seen {
		page, err := p.signatures(ctx, account.Address, "", "", slot, calls)
		if err != nil {
			return nil, err
		}
		entry := watchSeed{state: account}
		if len(page) > 0 {
			entry.high, entry.highSlot = page[0].Signature, page[0].Slot
		}
		seeds = append(seeds, entry)
	}
	return seeds, nil
}

func (p *DepositWatch) persistSeeds(
	ctx context.Context,
	wallet identity.MemberWallet,
	canonical chain.SolanaAddress,
	seeds []watchSeed,
	slot int64,
) error {
	return p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		if err := q.InsertDepositWatchWallet(
			ctx,
			sqlc.InsertDepositWatchWalletParams{
				WalletAddress: string(wallet.Address),
				UserID:        wallet.UserID.UUID(),
				FirstSeenSlot: slot,
				FirstSeenAt:   p.clock.Now(),
			},
		); err != nil {
			return err
		}
		for _, entry := range seeds {
			highSlot, err := intSlot(entry.highSlot)
			if err != nil {
				return err
			}
			state := "missing"
			if entry.state.Exists {
				state = "open"
			}
			if err := q.InsertDepositWatchAccount(
				ctx,
				sqlc.InsertDepositWatchAccountParams{
					TokenAccount:  string(entry.state.Address),
					WalletAddress: string(wallet.Address),
					Canonical:     entry.state.Address == canonical,
					State:         state,
					LastAmount:    entry.state.Amount.String(),
					ObservedSlot:  slot,
					HighSignature: string(entry.high),
					HighSlot:      highSlot,
					RecoveryDueAt: p.clock.Now(),
				},
			); err != nil {
				return err
			}
		}
		return nil
	})
}

func (p *DepositWatch) signatures(
	ctx context.Context,
	address chain.SolanaAddress,
	before, until chain.Signature,
	minSlot uint64,
	calls *int32,
) ([]solana.SignatureInfo, error) {
	if err := p.wait(ctx); err != nil {
		return nil, err
	}
	*calls++
	page, err := p.rpc.SignaturesFor(
		ctx,
		address,
		solana.SignaturesOpts{Before: before, Until: until, Limit: depositSignaturePageSize, MinContextSlot: minSlot},
	)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeRPCUnavailable, "funding.DepositWatch.signatures")
	}
	return page, nil
}

func (p *DepositWatch) wait(ctx context.Context) error {
	if err := p.limit.Wait(ctx); err != nil {
		return errs.Wrap(err, errs.CodeInternal, "funding.DepositWatch.wait")
	}
	return nil
}

func intSlot(value uint64) (int64, error) {
	if value > math.MaxInt64 {
		return 0, errs.New(errs.CodeInternal, "funding.DepositWatch.seed")
	}
	return int64(value), nil
}
