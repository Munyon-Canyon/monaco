package adapters

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type TokenBalances interface {
	TokenBalance(context.Context, chain.SolanaAddress, chain.Mint) (money.BaseUnits, error)
}

type Outflows struct {
	Funds       app.Outflows
	Withdrawals app.Outflows
}

type Balances struct {
	wallets  app.MemberWallets
	newRPC   func() TokenBalances
	rpc      TokenBalances
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
	newRPC func() TokenBalances,
	outflows Outflows,
	clk clock.Clock,
	usdc chain.Mint,
) *Balances {
	return &Balances{
		wallets: wallets, newRPC: newRPC, outflows: outflows, clock: clk, usdc: usdc,
		readings: map[ids.UserID]reading{},
	}
}

const displayMaxAge = 10 * time.Minute

type reading struct {
	onChain money.Micros
	at      time.Time
}

func (b *Balances) Available(ctx context.Context, user ids.UserID) (port.Balance, error) {
	const op = "funding.Balances.Available"
	address, err := b.wallets.MemberWalletAddress(ctx, user)
	if err != nil {
		return port.Balance{}, err
	}
	b.once.Do(func() { b.rpc = b.newRPC() })
	onChain, err := b.rpc.TokenBalance(ctx, address, b.usdc)
	if err != nil {
		return port.Balance{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if onChain.Decimals() != b.usdc.Decimals {
		return port.Balance{}, errs.New(errs.CodeDecodeFailed, op)
	}
	now := b.clock.Now()
	b.record(user, reading{onChain: onChainMicros(onChain), at: now})
	return b.compose(ctx, user, onChainMicros(onChain), now)
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
	return b.compose(ctx, user, last.onChain, last.at)
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

func (b *Balances) compose(
	ctx context.Context, user ids.UserID, onChain money.Micros, asOf time.Time,
) (port.Balance, error) {
	const op = "funding.Balances.compose"
	fund, err := b.outflows.Funds.InFlightMicros(ctx, user)
	if err != nil {
		return port.Balance{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	withdrawals, err := b.outflows.Withdrawals.InFlightMicros(ctx, user)
	if err != nil {
		return port.Balance{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	available, err := spendable(onChain, fund, withdrawals)
	if err != nil {
		observability.Degraded(ctx, observability.FundingBalanceClamped,
			slog.String("user_id", user.String()), slog.String("on_chain_micros", onChain.String()),
			slog.String("in_flight_fund_micros", fund.String()),
			slog.String("in_flight_withdrawal_micros", withdrawals.String()))
	}
	return port.Balance{
		OnChainMicros: onChain, InFlightFundMicros: fund, InFlightWithdrawalMicros: withdrawals,
		AvailableMicros: available, AsOf: asOf,
	}, nil
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
