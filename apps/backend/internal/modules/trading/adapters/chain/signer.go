package chain

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
)

type Privy interface {
	SignTransaction(ctx context.Context, walletID string, unsigned []byte) ([]byte, error)
}

type Signer struct {
	privy Privy
}

var (
	_ app.Signer = Signer{}
	_ Privy      = (*privy.Client)(nil)
)

func NewSigner(p Privy) Signer { return Signer{privy: p} }

func (s Signer) Sign(
	ctx context.Context, privyWalletID string, unsigned []byte,
) ([]byte, platform.Signature, error) {
	const op = "trading.Signer.Sign"
	signed, err := s.privy.SignTransaction(ctx, privyWalletID, unsigned)
	if err != nil {
		return nil, "", err
	}
	tx, err := platform.DecodeTransaction(signed)
	if err != nil {
		return nil, "", errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("wallet_id", privyWalletID))
	}
	if !tx.Signed(0) {
		return nil, "", errs.New(errs.CodeDecodeFailed, op, slog.String("wallet_id", privyWalletID),
			slog.String("reason", "first signature missing"))
	}
	return signed, platform.SignatureOf(tx.Signatures[0]), nil
}
