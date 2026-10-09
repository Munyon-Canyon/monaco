package app

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"math/big"
	"slices"
	"time"

	"go.opentelemetry.io/otel/metric"
	"golang.org/x/time/rate"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	DepositPollInterval      = 30 * time.Second
	depositSignaturePageSize = 1000
	depositWatchDirtyBatch   = 500
	depositWatchStopBefore   = 5 * time.Second
	depositWatchStopDivisor  = 5
	depositWatchClockSkew    = 5 * time.Minute

	depositCandidateSourcePoller = "poller"
)

var errRateBudgetSpent = errs.New(errs.CodeInternal, "funding.DepositWatch.rateBudgetSpent")

type RPCLimiter interface {
	Wait(context.Context) error
}

func NewRPCLimiter(perSecond int32) *rate.Limiter {
	if perSecond <= 0 {
		perSecond = 20
	}
	return rate.NewLimiter(rate.Limit(perSecond), int(perSecond))
}

type watchBudget struct {
	left, used int
	stopAt     time.Time
}

type watchBudgetKey struct{}

func newWatchBudget(ctx context.Context, calls int, period time.Duration) *watchBudget {
	b := &watchBudget{left: calls}
	if deadline, ok := ctx.Deadline(); ok {
		b.stopAt = deadline.Add(-min(depositWatchStopBefore, period/depositWatchStopDivisor))
	}
	return b
}

func (b *watchBudget) spent(now time.Time) bool {
	return b.left <= 0 || (!b.stopAt.IsZero() && !now.Before(b.stopAt))
}

type watchStep struct {
	name string
	run  func(context.Context) (scanned, recorded int, err error)
}

type DepositWatchRPC interface {
	SignaturesFor(
		context.Context, chain.SolanaAddress, solana.SignaturesOpts,
	) ([]solana.SignatureInfo, error)
	TokenAccounts(
		context.Context, chain.SolanaAddress, chain.Mint,
	) (uint64, []solana.TokenAccountState, error)
	Accounts(
		context.Context, []chain.SolanaAddress, uint64,
	) (uint64, []solana.TokenAccountState, error)
}

type DepositWatch struct {
	reads   sqlc.DBTX
	uow     *db.UnitOfWork
	ids     ids.Generator
	clock   clock.Clock
	wallets port.WalletReader
	rpc     DepositWatchRPC
	usdc    chain.SolanaAddress
	period  time.Duration
	limit   RPCLimiter
	calls   int
	tuning  DepositWatchTuning
	ledger  WalletLedger

	regressed metric.Int64Counter
}

func NewDepositWatch(
	reads sqlc.DBTX, uow *db.UnitOfWork, g ids.Generator, c clock.Clock, wallets port.WalletReader,
	rpc DepositWatchRPC, usdc chain.SolanaAddress, period time.Duration, limit RPCLimiter,
	callBudget int, tuning DepositWatchTuning, regressed metric.Int64Counter, ledger WalletLedger,
) *DepositWatch {
	return &DepositWatch{
		reads: reads, uow: uow, ids: g, clock: c, wallets: wallets, rpc: rpc, usdc: usdc,
		period: period, limit: limit, calls: callBudget, tuning: tuning,
		regressed: regressed, ledger: ledger,
	}
}

func (*DepositWatch) Name() string { return "funding.deposit_watch" }

func (p *DepositWatch) Interval() time.Duration { return p.period }

func (p *DepositWatch) Tick(ctx context.Context) (poller.Report, error) {
	budget := newWatchBudget(ctx, p.calls, p.period)
	ctx = context.WithValue(ctx, watchBudgetKey{}, budget)
	steps := []watchStep{
		{"gate", p.gate},
		{"dirty", p.catchUpDirty},
		{"first_sight", func(ctx context.Context) (int, int, error) {
			seeded, err := p.firstSight(ctx)
			return seeded, 0, err
		}},
		{"rotation", p.rotate},
		{"discovery", p.discover},
		{"reconcile", func(ctx context.Context) (int, int, error) { return p.reconcile(ctx, budget) }},
	}
	var report poller.Report
	var errList []error
	for _, step := range steps {
		before := budget.used
		scanned, recorded, err := step.run(ctx)
		report.Scanned += scanned
		report.Changed += recorded
		report.Attrs = append(report.Attrs,
			slog.Int(step.name, scanned), slog.Int(step.name+"_calls", budget.used-before))
		if errors.Is(err, errRateBudgetSpent) {
			break
		}
		errList = append(errList, err)
	}
	return report, errors.Join(errList...)
}

func (p *DepositWatch) catchUpDirty(ctx context.Context) (int, int, error) {
	rows, err := sqlc.New(p.reads).DepositWatchDirtyAccounts(ctx, depositWatchDirtyBatch)
	if err != nil {
		return 0, 0, errs.Wrap(err, errs.CodeOf(err), "funding.DepositWatch.dirty")
	}
	var scanned, recorded int
	for _, row := range rows {
		n, err := p.catchUpAccount(ctx, row)
		recorded += n
		if err != nil {
			return scanned, recorded, err
		}
		scanned++
	}
	return scanned, recorded, nil
}

func (p *DepositWatch) catchUpAccount(ctx context.Context, row sqlc.DepositWatchDirtyAccountsRow) (int, error) {
	opts := solana.SignaturesOpts{
		Before:         chain.Signature(row.PageBefore.String),
		Until:          chain.Signature(row.HighSignature.String),
		Limit:          depositSignaturePageSize,
		MinContextSlot: uint64(max(row.DirtySlot, row.ObservedSlot, 0)),
	}
	var recorded int
	for {
		page, err := p.signatures(ctx, chain.SolanaAddress(row.TokenAccount), opts)
		if err != nil {
			return recorded, err
		}
		if row.HistoryFloor.Valid {
			page, _ = cutBeforeTime(page, row.HistoryFloor.Time)
		} else if !row.HighSignature.Valid {
			page, _ = cutBelow(page, row.FirstSeenSlot)
		}
		n, err := p.commitPage(ctx, row, page)
		recorded += n
		if err != nil || len(page) < depositSignaturePageSize {
			return recorded, err
		}
		opts.Before = page[len(page)-1].Signature
	}
}

func (p *DepositWatch) commitPage(
	ctx context.Context, row sqlc.DepositWatchDirtyAccountsRow, page []solana.SignatureInfo,
) (int, error) {
	candidates, err := watchCandidates(row, page)
	if err != nil {
		return 0, err
	}
	var recorded int
	err = p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		if recorded, err = NewCandidateRecorder(p.ids, p.clock.Now).Record(ctx, tx, candidates); err != nil {
			return err
		}
		faultpoint.Hit(ctx, faultpoint.AfterCandidate)
		q := sqlc.New(tx.Queries())
		if len(page) > 0 {
			err := q.CheckpointDepositWatchPage(ctx, sqlc.CheckpointDepositWatchPageParams{
				TokenAccount: row.TokenAccount, PageBefore: string(page[len(page)-1].Signature),
				PageTopSignature: string(page[0].Signature), PageTopSlot: int64(min(page[0].Slot, math.MaxInt64)),
				ScannedAt: p.clock.Now(),
			})
			if err != nil {
				return err
			}
		}
		if len(page) == depositSignaturePageSize {
			return nil
		}
		return q.CompleteDepositWatchPage(ctx, sqlc.CompleteDepositWatchPageParams{
			TokenAccount: row.TokenAccount, CleanGen: row.DirtyGen, ScannedAt: p.clock.Now(),
		})
	})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), "funding.DepositWatch.commitPage")
	}
	return recorded, nil
}

func watchCandidates(row sqlc.DepositWatchDirtyAccountsRow, page []solana.SignatureInfo) ([]DepositCandidate, error) {
	return candidatesFor(row.WalletAddress, ids.UserIDFrom(row.UserID), depositCandidateSourcePoller, page)
}

func candidatesFor(
	wallet string, user ids.UserID, source string, page []solana.SignatureInfo,
) ([]DepositCandidate, error) {
	candidates := make([]DepositCandidate, 0, len(page))
	for _, sig := range page {
		if sig.Slot > math.MaxInt64 {
			return nil, errs.New(errs.CodeInternal, "funding.DepositWatch.candidates")
		}
		if sig.Failed {
			continue
		}
		candidates = append(candidates, DepositCandidate{
			Signature: sig.Signature, Wallet: chain.SolanaAddress(wallet),
			UserID: user, Slot: int64(sig.Slot), BlockTime: sig.BlockTime, Source: source,
		})
	}
	return candidates, nil
}

func (p *DepositWatch) firstSight(ctx context.Context) (int, error) {
	var after ids.UserID
	seeded := 0
	for {
		page, err := p.wallets.MemberWallets(ctx, after, port.MaxWalletPage)
		if err != nil {
			return seeded, errs.Wrap(err, errs.CodeOf(err), "funding.DepositWatch.firstSight")
		}
		n, err := p.seedUnknown(ctx, page)
		seeded += n
		if err != nil || len(page) < port.MaxWalletPage {
			return seeded, err
		}
		after = page[len(page)-1].UserID
	}
}

func (p *DepositWatch) seedUnknown(ctx context.Context, page []port.MemberWallet) (int, error) {
	addresses := make([]string, len(page))
	for i, wallet := range page {
		addresses[i] = string(wallet.Address)
	}
	known, err := sqlc.New(p.reads).DepositWatchKnownWallets(ctx, addresses)
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), "funding.DepositWatch.firstSight")
	}
	seeded := 0
	for _, wallet := range page {
		if slices.Contains(known, string(wallet.Address)) {
			continue
		}
		if err := p.seedWallet(ctx, wallet); err != nil {
			return seeded, err
		}
		seeded++
	}
	return seeded, nil
}

type watchSeed struct {
	state solana.TokenAccountState
	tipScan
}

type tipScan struct {
	high   solana.SignatureInfo
	window []solana.SignatureInfo
	resume chain.Signature
}

func (p *DepositWatch) seedWallet(ctx context.Context, wallet port.MemberWallet) error {
	if err := p.wait(ctx); err != nil {
		return err
	}
	slot, accounts, err := p.rpc.TokenAccounts(
		ctx, wallet.Address, chain.Mint{Address: p.usdc, Decimals: usdcDecimals},
	)
	if err != nil {
		return errs.Wrap(err, errs.CodeRPCUnavailable, "funding.DepositWatch.tokenAccounts")
	}
	if slot > math.MaxInt64 {
		return errs.New(errs.CodeInternal, "funding.DepositWatch.seed")
	}
	canonical, err := chain.AssociatedTokenAccount(wallet.Address, p.usdc, chain.SPLProgram)
	if err != nil {
		return errs.Wrap(err, errs.CodeInvalidAddress, "funding.DepositWatch.seed")
	}
	states := map[chain.SolanaAddress]solana.TokenAccountState{canonical: {Address: canonical}}
	for _, account := range accounts {
		states[account.Address] = account
	}
	floor := p.firstSightFloor(wallet)
	seeds, candidates, pending, err := p.scanStates(ctx, wallet, states, slot, floor)
	if err != nil {
		return err
	}
	opening := ""
	if !pending {
		if opening, err = p.openingMicros(ctx, wallet, seeds); err != nil {
			return err
		}
	}
	return p.persistSeeds(ctx, wallet, seedPlan{
		canonical: canonical, seeds: seeds, candidates: candidates, slot: int64(slot), opening: opening, floor: floor,
	})
}

func (p *DepositWatch) firstSightFloor(wallet port.MemberWallet) time.Time {
	if wallet.CreatedAt.IsZero() {
		return p.clock.Now()
	}
	return wallet.CreatedAt.Add(-depositWatchClockSkew)
}

func (p *DepositWatch) scanStates(
	ctx context.Context, wallet port.MemberWallet, states map[chain.SolanaAddress]solana.TokenAccountState,
	slot uint64, floor time.Time,
) ([]watchSeed, []DepositCandidate, bool, error) {
	seeds := make([]watchSeed, 0, len(states))
	var candidates []DepositCandidate
	pending := false
	for _, state := range states {
		scan, err := p.scanFromTip(ctx, state.Address, slot, floor)
		if err != nil {
			return nil, nil, false, err
		}
		found := windowCandidates(wallet, scan.window)
		candidates = append(candidates, found...)
		pending = pending || len(found) > 0 || scan.resume != ""
		seeds = append(seeds, watchSeed{state: state, tipScan: scan})
	}
	return seeds, candidates, pending, nil
}

func (p *DepositWatch) openingMicros(ctx context.Context, wallet port.MemberWallet, seeds []watchSeed) (string, error) {
	total := new(big.Int)
	for _, seed := range seeds {
		total.Add(total, wholeMicros(seed.state.Amount.String()))
	}
	settled, _, err := p.ledger.WalletLedgerMicros(ctx, wallet.UserID, p.usdc)
	if err != nil {
		return "", errs.Wrap(err, errs.CodeOf(err), "funding.DepositWatch.opening")
	}
	return total.Sub(total, wholeMicros(settled.String())).String(), nil
}

func (p *DepositWatch) scanFromTip(
	ctx context.Context, account chain.SolanaAddress, slot uint64, floor time.Time,
) (tipScan, error) {
	var scan tipScan
	opts := solana.SignaturesOpts{Limit: depositSignaturePageSize, MinContextSlot: slot}
	for {
		page, err := p.signatures(ctx, account, opts)
		if err != nil {
			return tipScan{}, err
		}
		atOrBelow := slices.DeleteFunc(slices.Clone(page), func(sig solana.SignatureInfo) bool {
			return sig.Slot > slot
		})
		if scan.high.Signature == "" && len(atOrBelow) > 0 {
			scan.high = atOrBelow[0]
		}
		window, cut := cutBeforeTime(atOrBelow, floor)
		scan.window = append(scan.window, window...)
		if cut || len(page) < depositSignaturePageSize {
			return scan, nil
		}
		opts.Before = page[len(page)-1].Signature
		if scan.high.Signature != "" {
			scan.resume = opts.Before
			return scan, nil
		}
	}
}

func windowCandidates(wallet port.MemberWallet, window []solana.SignatureInfo) []DepositCandidate {
	candidates := make([]DepositCandidate, 0, len(window))
	for _, sig := range window {
		if sig.Failed {
			continue
		}
		candidates = append(candidates, DepositCandidate{
			Signature: sig.Signature, Wallet: wallet.Address, UserID: wallet.UserID,
			Slot: int64(min(sig.Slot, math.MaxInt64)), BlockTime: sig.BlockTime, Source: depositCandidateSourcePoller,
		})
	}
	return candidates
}

func cutBeforeTime(page []solana.SignatureInfo, floor time.Time) ([]solana.SignatureInfo, bool) {
	i := slices.IndexFunc(page, func(sig solana.SignatureInfo) bool { return sig.BlockTime.Before(floor) })
	if i < 0 {
		return page, false
	}
	return page[:i], true
}

type seedPlan struct {
	canonical  chain.SolanaAddress
	seeds      []watchSeed
	candidates []DepositCandidate
	slot       int64
	opening    string
	floor      time.Time
}

func (p *DepositWatch) seedAccount(
	wallet port.MemberWallet, plan seedPlan, seed watchSeed,
) sqlc.InsertDepositWatchAccountParams {
	state := "missing"
	if seed.state.Exists {
		state = "open"
	}
	params := sqlc.InsertDepositWatchAccountParams{
		TokenAccount:  string(seed.state.Address),
		WalletAddress: string(wallet.Address),
		Canonical:     seed.state.Address == plan.canonical,
		State:         state,
		LastAmount:    seed.state.Amount.String(),
		ObservedSlot:  plan.slot,
		HighSignature: string(seed.high.Signature),
		HighSlot:      int64(min(seed.high.Slot, math.MaxInt64)),
		RecoveryDueAt: p.clock.Now().Add(p.tuning.Spread(p.tuning.Rotation)),
	}
	if seed.resume != "" {
		params.HighSignature, params.HighSlot = "", 0
		params.DirtyGen, params.DirtySlot = 1, plan.slot
		params.PageBefore = string(seed.resume)
		params.PageTopSignature = string(seed.high.Signature)
		params.PageTopSlot = int64(min(seed.high.Slot, math.MaxInt64))
		params.HistoryFloor = plan.floor
	}
	return params
}

func (p *DepositWatch) persistSeeds(ctx context.Context, wallet port.MemberWallet, plan seedPlan) error {
	slot := plan.slot
	err := p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		if _, err := q.InsertDepositWatchWallet(ctx, sqlc.InsertDepositWatchWalletParams{
			WalletAddress: string(wallet.Address), UserID: wallet.UserID.UUID(), FirstSeenSlot: slot,
			FirstSeenAt: p.clock.Now(), DiscoveryDueAt: p.clock.Now().Add(p.tuning.Spread(p.tuning.Discovery)),
			OpeningMicros: plan.opening, ReconcileDueAt: p.clock.Now().Add(p.tuning.Spread(DepositResidualInterval)),
		}); err != nil {
			return err
		}
		for _, seed := range plan.seeds {
			if err := q.InsertDepositWatchAccount(ctx, p.seedAccount(wallet, plan, seed)); err != nil {
				return err
			}
		}
		if _, err := NewCandidateRecorder(p.ids, p.clock.Now).Record(ctx, tx, plan.candidates); err != nil {
			return err
		}
		faultpoint.Hit(ctx, faultpoint.AfterCandidate)
		tx.AfterCommit(func(ctx context.Context) { faultpoint.Hit(ctx, faultpoint.AfterCandidate) })
		return nil
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeOf(err), "funding.DepositWatch.persistSeeds")
	}
	return nil
}

func (p *DepositWatch) signatures(
	ctx context.Context, account chain.SolanaAddress, opts solana.SignaturesOpts,
) ([]solana.SignatureInfo, error) {
	if err := p.wait(ctx); err != nil {
		return nil, err
	}
	page, err := p.rpc.SignaturesFor(ctx, account, opts)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeRPCUnavailable, "funding.DepositWatch.signatures")
	}
	return page, nil
}

func (p *DepositWatch) wait(ctx context.Context) error {
	budget, _ := ctx.Value(watchBudgetKey{}).(*watchBudget)
	if budget != nil && budget.spent(p.clock.Now()) {
		return errRateBudgetSpent
	}
	err := p.limit.Wait(ctx)
	if err == nil {
		if budget != nil {
			budget.left--
			budget.used++
		}
		return nil
	}
	if ctx.Err() == nil {
		return errRateBudgetSpent
	}
	return errs.Wrap(err, errs.CodeInternal, "funding.DepositWatch.wait")
}
