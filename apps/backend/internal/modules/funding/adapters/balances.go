package adapters

import (
	"context"
	"log/slog"
	"sync"

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
	return &Balances{wallets: wallets, newRPC: newRPC, outflows: outflows, clock: clk, usdc: usdc}
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
	fund, err := b.outflows.Funds.InFlightMicros(ctx, user)
	if err != nil {
		return port.Balance{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	withdrawals, err := b.outflows.Withdrawals.InFlightMicros(ctx, user)
	if err != nil {
		return port.Balance{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	available, err := spendable(onChainMicros(onChain), fund, withdrawals)
	if err != nil {
		observability.Degraded(ctx, observability.FundingBalanceClamped,
			slog.String("user_id", user.String()), slog.String("on_chain_micros", onChain.String()),
			slog.String("in_flight_fund_micros", fund.String()),
			slog.String("in_flight_withdrawal_micros", withdrawals.String()))
	}
	return port.Balance{
		OnChainMicros: onChainMicros(onChain), InFlightFundMicros: fund, InFlightWithdrawalMicros: withdrawals,
		AvailableMicros: available, AsOf: b.clock.Now(),
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
