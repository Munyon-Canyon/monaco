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
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type ChainReads interface {
	TokenBalancesAt(
		ctx context.Context, owner chain.SolanaAddress, mint chain.Mint, commitment string,
	) (solana.TokenBalances, error)
	app.SignatureStatuses
}

type Outflows struct {
	Funds         app.Outflows
	Withdrawals   app.Outflows
	FundsOn       func(dbsqlc.DBTX) app.Outflows
	WithdrawalsOn func(dbsqlc.DBTX) app.Outflows
}

func (o Outflows) on(q dbsqlc.DBTX) Outflows {
	bound := Outflows{Funds: o.Funds, Withdrawals: o.Withdrawals}
	if o.FundsOn != nil {
		bound.Funds = o.FundsOn(q)
	}
	if o.WithdrawalsOn != nil {
		bound.Withdrawals = o.WithdrawalsOn(q)
	}
	return bound
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
	observationTTL = 5 * time.Second
)

type reading struct {
	onChain     money.Micros
	spendable   money.Micros
	unspendable money.Micros
	slot        uint64
	at          time.Time
}

type outflowReading struct {
	funds, sent inFlight
	read        bool
}

func (b *Balances) Available(ctx context.Context, user ids.UserID) (port.Balance, error) {
	obs, flows, err := b.observe(ctx, user)
	if err != nil {
		return port.Balance{}, err
	}
	return b.compose(ctx, obs, flows.funds, flows.sent), nil
}

func (b *Balances) Observe(ctx context.Context, user ids.UserID) (port.Observation, error) {
	obs, _, err := b.observe(ctx, user)
	return obs, err
}

func (b *Balances) observe(ctx context.Context, user ids.UserID) (port.Observation, outflowReading, error) {
	const op = "funding.Balances.Observe"
	address, err := b.wallets.MemberWalletAddress(ctx, user)
	if err != nil {
		return port.Observation{}, outflowReading{}, err
	}
	funds, sent, err := readOutflows(ctx, b.outflows, user)
	if err != nil {
		return port.Observation{}, outflowReading{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	flows := outflowReading{funds: funds, sent: sent, read: true}
	b.once.Do(func() { b.rpc = b.newRPC() })
	got, err := b.rpc.TokenBalancesAt(ctx, address, b.usdc, inFlightEndsAt)
	if err != nil {
		return port.Observation{}, flows, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if got.Total.Decimals() != b.usdc.Decimals || got.Spendable.Decimals() != b.usdc.Decimals ||
		got.Unspendable.Decimals() != b.usdc.Decimals {
		return port.Observation{}, flows, errs.New(errs.CodeDecodeFailed, op)
	}
	r := reading{
		onChain: onChainMicros(got.Total), spendable: onChainMicros(got.Spendable),
		unspendable: onChainMicros(got.Unspendable),
		slot:        got.Slot, at: b.clock.Now(),
	}
	b.record(user, r)
	obs := port.Observation{
		User: user, OnChainMicros: r.onChain, SpendableMicros: r.spendable, UnspendableMicros: r.unspendable,
		Slot: r.slot, At: r.at,
		Signatures: b.landed(ctx, r.slot, funds, sent),
	}
	return obs, flows, nil
}

func (b *Balances) AvailableIn(ctx context.Context, q dbsqlc.DBTX, obs port.Observation) (port.Balance, error) {
	const op = "funding.Balances.AvailableIn"
	funds, sent, err := readOutflows(ctx, b.outflows.on(q), obs.User)
	if err != nil {
		return port.Balance{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if b.clock.Now().Sub(obs.At) > observationTTL || settledSince(obs, funds, sent) {
		return port.Balance{}, errs.New(errs.CodeRPCUnavailable, op, slog.String("reason", "balance_reading_stale"))
	}
	return b.compose(ctx, obs, funds, sent), nil
}

func (b *Balances) ForDisplay(ctx context.Context, user ids.UserID) (port.Balance, error) {
	obs, flows, err := b.observe(ctx, user)
	if err == nil {
		return b.compose(ctx, obs, flows.funds, flows.sent), nil
	}
	if errs.CodeOf(err) != errs.CodeRPCUnavailable || !flows.read {
		return port.Balance{}, err
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
	cached := port.Observation{
		User: user, OnChainMicros: last.onChain, SpendableMicros: last.spendable,
		UnspendableMicros: last.unspendable, Slot: last.slot, At: last.at,
	}
	return b.compose(ctx, cached, flows.funds, flows.sent), nil
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

func (b *Balances) compose(ctx context.Context, obs port.Observation, funds, sent inFlight) port.Balance {
	fund, withdrawals := funds.without(obs.Signatures), sent.without(obs.Signatures)
	available, err := spendable(obs.SpendableMicros, fund, withdrawals)
	if err != nil {
		observability.Degraded(ctx, observability.FundingBalanceClamped,
			slog.String("user_id", obs.User.String()), slog.String("on_chain_micros", obs.OnChainMicros.String()),
			slog.String("in_flight_fund_micros", fund.String()),
			slog.String("in_flight_withdrawal_micros", withdrawals.String()))
	}
	return port.Balance{
		OnChainMicros: obs.OnChainMicros, InFlightFundMicros: fund, InFlightWithdrawalMicros: withdrawals,
		AvailableMicros: available, UnspendableMicros: obs.UnspendableMicros, AsOf: obs.At,
	}
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

func readOutflows(ctx context.Context, outflows Outflows, user ids.UserID) (funds, sent inFlight, err error) {
	if funds, err = readInFlight(ctx, outflows.Funds, user); err != nil {
		return inFlight{}, inFlight{}, err
	}
	if sent, err = readInFlight(ctx, outflows.Withdrawals, user); err != nil {
		return inFlight{}, inFlight{}, err
	}
	return funds, sent, nil
}

func (b *Balances) landed(ctx context.Context, slot uint64, outflows ...inFlight) map[chain.Signature]bool {
	var sigs []chain.Signature
	for _, o := range outflows {
		sigs = slices.AppendSeq(sigs, maps.Keys(o.submitted))
	}
	landed := make(map[chain.Signature]bool, len(sigs))
	for _, sig := range sigs {
		landed[sig] = false
	}
	if len(sigs) == 0 {
		return landed
	}
	statuses, err := b.rpc.SignatureStatuses(ctx, sigs)
	if err != nil {
		return landed
	}
	for _, s := range statuses {
		landed[s.Signature] = s.State == solana.StateFinalized && s.Slot <= slot
	}
	return landed
}

func settledSince(obs port.Observation, outflows ...inFlight) bool {
	for sig, landed := range obs.Signatures {
		if landed {
			continue
		}
		if !slices.ContainsFunc(outflows, func(o inFlight) bool { _, ok := o.submitted[sig]; return ok }) {
			return true
		}
	}
	return false
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
