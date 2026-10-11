package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type TransferQuote struct {
	ReceiveMicros money.Micros
	ExpiresAt     time.Time
}

type TransferQuotes interface {
	QuoteTransfer(ctx context.Context, walletID string, spec multichain.TransferSpec) (TransferQuote, error)
}

type WithdrawalPreview struct {
	ReceiveMicros money.Micros
	ExpiresAt     *time.Time
}

type PreviewWithdrawalHandler struct {
	Wallets SigningWallets
	Quotes  TransferQuotes
}

func (h PreviewWithdrawalHandler) Handle(
	ctx context.Context, user ids.UserID, r domain.WithdrawalRequest,
) (WithdrawalPreview, error) {
	if r.Kind() == domain.DestinationSolana {
		return WithdrawalPreview{ReceiveMicros: r.Amount}, nil
	}
	wallet, err := h.Wallets.SigningWallet(ctx, user)
	if err != nil {
		return WithdrawalPreview{}, err
	}
	quote, err := h.Quotes.QuoteTransfer(ctx, wallet.ID, evmTransferSpec(r))
	if err != nil {
		return WithdrawalPreview{}, err
	}
	return WithdrawalPreview{ReceiveMicros: quote.ReceiveMicros, ExpiresAt: &quote.ExpiresAt}, nil
}
