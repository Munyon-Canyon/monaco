package chain

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
)

type Privy interface {
	SignTransaction(ctx context.Context, walletID string, unsigned []byte) ([]byte, error)
}

type CoSigner interface {
	Address() platform.SolanaAddress
	CoSign(ctx context.Context, raw []byte) ([]byte, error)
}

type Signer struct {
	privy   Privy
	relayer CoSigner
}

var (
	_ app.Signer = Signer{}
	_ Privy      = (*privy.Client)(nil)
	_ CoSigner   = (*relayer.Relayer)(nil)
)

func NewSigner(p Privy, r CoSigner) Signer { return Signer{privy: p, relayer: r} }

func (s Signer) FeePayer() (platform.SolanaAddress, error) { return s.relayer.Address(), nil }

func (s Signer) Sign(
	ctx context.Context, privyWalletID string, unsigned []byte,
) ([]byte, platform.Signature, error) {
	const op = "trading.Signer.Sign"
	walletSigned, err := s.privy.SignTransaction(ctx, privyWalletID, unsigned)
	if err != nil {
		return nil, "", err
	}
	signed, err := s.relayer.CoSign(ctx, walletSigned)
	if err != nil {
		return nil, "", errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("wallet_id", privyWalletID))
	}
	tx, err := platform.DecodeTransaction(signed)
	if err != nil {
		return nil, "", errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("wallet_id", privyWalletID))
	}
	if !tx.Signed(0) || !walletSignedASlotAfterTheFeePayer(tx) {
		return nil, "", errs.New(errs.CodeDecodeFailed, op, slog.String("wallet_id", privyWalletID),
			slog.String("reason", "fee payer or treasury signature missing"))
	}
	return signed, platform.SignatureOf(tx.Signatures[0]), nil
}

func walletSignedASlotAfterTheFeePayer(tx platform.Transaction) bool {
	for i := 1; i < len(tx.Signatures); i++ {
		if tx.Signed(i) {
			return true
		}
	}
	return false
}
