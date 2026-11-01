package chain

import (
	"context"
	"log/slog"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	platform "github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Solana interface {
	SignatureStatuses(context.Context, []platform.Signature) ([]solana.Status, error)
	Received(context.Context, platform.Signature, platform.SolanaAddress, platform.Mint) (money.BaseUnits, error)
	BlockhashValid(context.Context, string) (bool, error)
	SignaturesFor(context.Context, platform.SolanaAddress, solana.SignaturesOpts) ([]solana.SignatureInfo, error)
	TransactionSignatures(context.Context, platform.Signature) ([]platform.Signature, error)
}

const findSignatureWindow = 25

type Reader struct {
	solana Solana
}

var (
	_ app.ChainReader = Reader{}
	_ Solana          = (*solana.Client)(nil)
)

func NewReader(s Solana) Reader { return Reader{solana: s} }

func (r Reader) BlockhashValid(ctx context.Context, signed []byte) (bool, error) {
	hash, err := platform.RecentBlockhash(signed)
	if err != nil {
		return false, err
	}
	return r.solana.BlockhashValid(ctx, hash)
}

func (r Reader) SignatureStatuses(ctx context.Context, sigs []platform.Signature) ([]app.SigStatus, error) {
	statuses, err := r.solana.SignatureStatuses(ctx, sigs)
	if err != nil {
		return nil, err
	}
	out := make([]app.SigStatus, len(statuses))
	for i, status := range statuses {
		state, ok := map[solana.State]app.SigState{
			solana.StateFinalized:  app.SigFinalized,
			solana.StateProcessing: app.SigProcessing,
			solana.StateNotFound:   app.SigNotFound,
		}[status.State]
		if !ok {
			return nil, errs.New(errs.CodeDecodeFailed, "trading.ChainReader.SignatureStatuses",
				slog.Int("state", int(status.State)))
		}
		out[i] = app.SigStatus{
			State: state, Failed: status.Failed, BlockHeight: status.BlockHeight, ContextSlot: status.ContextSlot,
		}
	}
	return out, nil
}

func (r Reader) InboundAmount(
	ctx context.Context,
	signature platform.Signature,
	owner platform.SolanaAddress,
	mint platform.Mint,
) (money.BaseUnits, error) {
	got, err := r.solana.Received(ctx, signature, owner, mint)
	if err == nil && got.IsZero() {
		err = errs.New(errs.CodeDecodeFailed, "trading.ChainReader.InboundAmount",
			slog.String("signature", string(signature)), slog.String("mint", string(mint.Address)),
			slog.String("reason", "the swap moved nothing into the treasury"))
	}
	return got, err
}

func (r Reader) FindSignature(
	ctx context.Context, owner platform.SolanaAddress, standIn platform.Signature,
) (platform.Signature, bool, error) {
	treasury, ok := platform.StandInTreasurySignature(standIn)
	if !ok {
		return "", false, errs.New(errs.CodeInvalidInput, "trading.ChainReader.FindSignature",
			slog.String("signature", string(standIn)))
	}
	recent, err := r.solana.SignaturesFor(ctx, owner, solana.SignaturesOpts{Limit: findSignatureWindow})
	if err != nil {
		return "", false, err
	}
	for _, info := range recent {
		signatures, err := r.solana.TransactionSignatures(ctx, info.Signature)
		switch {
		case errs.CodeOf(err) == errs.CodeNotFound:
			continue
		case err != nil:
			return "", false, err
		case slices.Contains(signatures, treasury):
			return info.Signature, true, nil
		}
	}
	return "", false, nil
}
