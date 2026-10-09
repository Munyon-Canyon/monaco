package adapters

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type ChainReads interface {
	TokenBalanceAt(
		ctx context.Context, owner chain.SolanaAddress, mint chain.Mint, commitment string,
	) (uint64, money.BaseUnits, error)
	app.SignatureStatuses
}

type Outflows struct {
	Funds       app.Outflows
	Withdrawals app.Outflows
}

type Balances struct {
	wallets  app.MemberWallets
	newRPC   func() ChainReads
	rpc      ChainReads
	once     sync.Once
	mu       sync.Mutex
	readings map[ids.UserID]reading
	outflows Outflows
	clock    clock.Clock
	usdc     chain.Mint
}

var _ port.Balances = (*Balances)(nil)

func NewBalances(
	wallets app.MemberWallets,
	newRPC func() ChainReads,
	outflows Outflows,
	clk clock.Clock,
	usdc chain.Mint,
) *Balances {
	return &Balances{
		wallets: wallets, newRPC: newRPC, outflows: outflows, clock: clk, usdc: usdc,
		readings: map[ids.UserID]reading{},
	}
}

const (
	displayMaxAge  = 10 * time.Minute
	inFlightEndsAt = "finalized"
)

type reading struct {
	onChain money.Micros
	slot    uint64
	at      time.Time
}

func (b *Balances) Available(ctx context.Context, user ids.UserID) (port.Balance, error) {
	const op = "funding.Balances.Available"
	address, err := b.wallets.MemberWalletAddress(ctx, user)
	if err != nil {
		return port.Balance{}, err
	}
	b.once.Do(func() { b.rpc = b.newRPC() })
	slot, onChain, err := b.rpc.TokenBalanceAt(ctx, address, b.usdc, inFlightEndsAt)
	if err != nil {
		return port.Balance{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if onChain.Decimals() != b.usdc.Decimals {
		return port.Balance{}, errs.New(errs.CodeDecodeFailed, op)
	}
	r := reading{onChain: onChainMicros(onChain), slot: slot, at: b.clock.Now()}
	b.record(user, r)
	return b.compose(ctx, user, r)
}

func (b *Balances) ForDisplay(ctx context.Context, user ids.UserID) (port.Balance, error) {
	got, err := b.Available(ctx, user)
	if err == nil || errs.CodeOf(err) != errs.CodeRPCUnavailable {
		return got, err
	}
	last, ok := b.lastReading(user)
	if !ok || b.clock.Now().Sub(last.at) >= displayMaxAge {
		return port.Balance{}, err
	}
	for _, outflows := range []app.Outflows{b.outflows.Funds, b.outflows.Withdrawals} {
		changed, cerr := outflows.LastChange(ctx, user)
		if cerr != nil {
			return port.Balance{}, errs.Wrap(cerr, errs.CodeOf(cerr), "funding.Balances.ForDisplay")
		}
		if !changed.Before(last.at) {
			return port.Balance{}, err
		}
	}
	return b.compose(ctx, user, last)
}

func (b *Balances) record(user ids.UserID, r reading) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.readings[user] = r
}

func (b *Balances) lastReading(user ids.UserID) (reading, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	r, ok := b.readings[user]
	return r, ok
}

func (b *Balances) compose(ctx context.Context, user ids.UserID, r reading) (port.Balance, error) {
	const op = "funding.Balances.compose"
	funds, err := readInFlight(ctx, b.outflows.Funds, user)
	if err != nil {
		return port.Balance{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	sent, err := readInFlight(ctx, b.outflows.Withdrawals, user)
	if err != nil {
		return port.Balance{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	landed := b.landed(ctx, r.slot, funds, sent)
	fund, withdrawals := funds.without(landed), sent.without(landed)
	available, err := spendable(r.onChain, fund, withdrawals)
	if err != nil {
		observability.Degraded(ctx, observability.FundingBalanceClamped,
			slog.String("user_id", user.String()), slog.String("on_chain_micros", r.onChain.String()),
			slog.String("in_flight_fund_micros", fund.String()),
			slog.String("in_flight_withdrawal_micros", withdrawals.String()))
	}
	return port.Balance{
		OnChainMicros: r.onChain, InFlightFundMicros: fund, InFlightWithdrawalMicros: withdrawals,
		AvailableMicros: available, AsOf: r.at,
	}, nil
}

type inFlight struct {
	total     money.Micros
	submitted map[chain.Signature]money.Micros
}

func readInFlight(ctx context.Context, outflows app.Outflows, user ids.UserID) (inFlight, error) {
	total, err := outflows.InFlightMicros(ctx, user)
	if err != nil {
		return inFlight{}, err
	}
	submitted, err := outflows.Submitted(ctx, user)
	return inFlight{total: total, submitted: submitted}, err
}

func (b *Balances) landed(ctx context.Context, slot uint64, outflows ...inFlight) map[chain.Signature]bool {
	var sigs []chain.Signature
	for _, o := range outflows {
		sigs = slices.AppendSeq(sigs, maps.Keys(o.submitted))
	}
	if len(sigs) == 0 {
		return nil
	}
	statuses, err := b.rpc.SignatureStatuses(ctx, sigs)
	if err != nil {
		return nil
	}
	landed := make(map[chain.Signature]bool, len(statuses))
	for _, s := range statuses {
		landed[s.Signature] = s.State == solana.StateFinalized && s.Slot <= slot
	}
	return landed
}

func (o inFlight) without(landed map[chain.Signature]bool) money.Micros {
	left := o.total
	for sig, amount := range o.submitted {
		if !landed[sig] {
			continue
		}
		next, err := left.Sub(amount)
		if err != nil {
			return o.total
		}
		left = next
	}
	return left
}

func spendable(onChain, fund, withdrawals money.Micros) (money.Micros, error) {
	afterFund, err := onChain.Sub(fund)
	if err != nil {
		return money.Micros{}, err
	}
	left, err := afterFund.Sub(withdrawals)
	if err != nil {
		return money.Micros{}, err
	}
	return left, nil
}

func onChainMicros(value money.BaseUnits) money.Micros { return money.MicrosFromUint64(value.Uint64()) }
