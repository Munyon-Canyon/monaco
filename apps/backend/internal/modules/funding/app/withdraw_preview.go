package app

import (
	"context"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const bridgeFeeLife = 5 * time.Minute

type TransferQuote struct {
	ReceiveMicros money.Micros
	ExpiresAt     time.Time
}

type TransferQuotes interface {
	QuoteTransfer(ctx context.Context, walletID string, spec multichain.TransferSpec) (TransferQuote, error)
}

type WithdrawalPreview struct {
	AvailableMicros money.Micros
	FeeMicros       *money.Micros
	FeeMode         domain.FeeMode
	MaxAmountMicros money.Micros
	ReceiveMicros   *money.Micros
	OverMicros      *money.Micros
	BelowMin        bool
	MinMicros       money.Micros
	ArrivalSeconds  int
	ExpiresAt       *time.Time
}

type bridgeFee struct {
	micros money.Micros
	seen   time.Time
}

type BridgeFees struct {
	clock clock.Clock
	mu    sync.Mutex
	seen  map[chain.Network]bridgeFee
}

func NewBridgeFees(clk clock.Clock) *BridgeFees {
	return &BridgeFees{clock: clk, seen: map[chain.Network]bridgeFee{}}
}

func (b *BridgeFees) get(network chain.Network) (money.Micros, bool) {
	if b == nil {
		return money.Micros{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	fee, ok := b.seen[network]
	if !ok || b.clock.Now().Sub(fee.seen) >= bridgeFeeLife {
		return money.Micros{}, false
	}
	return fee.micros, true
}

func (b *BridgeFees) put(network chain.Network, fee money.Micros) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seen[network] = bridgeFee{micros: fee, seen: b.clock.Now()}
}

type PreviewWithdrawalHandler struct {
	Wallets  SigningWallets
	Quotes   TransferQuotes
	Balances port.DisplayBalances
	Fees     *BridgeFees
}

func (h PreviewWithdrawalHandler) Handle(
	ctx context.Context, user ids.UserID, r domain.WithdrawalPreviewRequest,
) (WithdrawalPreview, error) {
	balance, err := h.Balances.ForDisplay(ctx, user)
	if err != nil {
		return WithdrawalPreview{}, err
	}
	fee, mode := domain.WithdrawalFee(r.Network)
	feeKnown := !r.Network.IsEVM()
	if !feeKnown {
		fee, feeKnown = h.Fees.get(r.Network)
	}
	out := WithdrawalPreview{
		AvailableMicros: balance.AvailableMicros,
		FeeMode:         mode,
		MaxAmountMicros: domain.MaxSendable(balance.AvailableMicros, fee, mode),
		MinMicros: money.MicrosFromUint64(
			domain.MinWithdrawalMicros,
		),
		ArrivalSeconds: domain.ArrivalSeconds(r.Network),
	}
	if feeKnown {
		out.FeeMicros = &fee
	}
	if r.Amount == nil {
		return out, nil
	}
	amount := *r.Amount
	over := overBy(amount, out.MaxAmountMicros)
	out.OverMicros = &over
	switch {
	case amount.Cmp(out.MinMicros) < 0:
		out.BelowMin = true
		out.OverMicros = nil
	case !over.IsZero():
	case !r.Network.IsEVM():
		out.ReceiveMicros = &amount
	case r.EVM != nil:
		return h.quote(ctx, user, out, domain.WithdrawalRequest{Amount: amount, Network: r.Network, EVM: *r.EVM})
	}
	return out, nil
}

func (h PreviewWithdrawalHandler) quote(
	ctx context.Context, user ids.UserID, out WithdrawalPreview, r domain.WithdrawalRequest,
) (WithdrawalPreview, error) {
	wallet, err := h.Wallets.SigningWallet(ctx, user)
	if err != nil {
		return WithdrawalPreview{}, err
	}
	quote, err := h.Quotes.QuoteTransfer(ctx, wallet.ID, evmTransferSpec(r))
	if err != nil {
		return WithdrawalPreview{}, err
	}
	fee := overBy(r.Amount, quote.ReceiveMicros)
	h.Fees.put(r.Network, fee)
	out.FeeMicros, out.ReceiveMicros, out.ExpiresAt = &fee, &quote.ReceiveMicros, &quote.ExpiresAt
	return out, nil
}

func overBy(amount, limit money.Micros) money.Micros {
	over, err := amount.Sub(limit)
	if err != nil {
		return money.Micros{}
	}
	return over
}
