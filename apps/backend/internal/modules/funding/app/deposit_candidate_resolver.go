package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type SignatureOwner interface {
	OwnsSignature(context.Context, chain.Signature) (bool, error)
}

type DepositTransferReader interface {
	InboundTransfersForMint(
		context.Context,
		chain.Signature,
		chain.SolanaAddress,
		chain.SolanaAddress,
	) ([]solana.Transfer, error)
}

type DepositCandidateResolver struct {
	reader  DepositTransferReader
	limit   RPCLimiter
	usdc    chain.SolanaAddress
	owners  []SignatureOwner
	credits *CreditDepositHandler
	ids     ids.Generator
}

type DepositCandidateResolution struct {
	Amount money.Micros
	Ours   bool
}

func NewDepositCandidateResolver(
	reader DepositTransferReader,
	limit RPCLimiter,
	usdc chain.SolanaAddress,
	owners []SignatureOwner,
	credits *CreditDepositHandler,
	g ids.Generator,
) DepositCandidateResolver {
	return DepositCandidateResolver{reader: reader, limit: limit, usdc: usdc, owners: owners, credits: credits, ids: g}
}

func (r DepositCandidateResolver) Fetch(
	ctx context.Context,
	e events.DepositCandidateSeen,
) (DepositCandidateResolution, error) {
	if err := r.limit.Wait(ctx); err != nil {
		err = errs.Wrap(err, errs.CodeRPCUnavailable, "funding.DepositCandidateResolver.Fetch")
		r.unresolved(ctx, e.WalletAddress, err)
		return DepositCandidateResolution{}, err
	}
	transfers, err := r.reader.InboundTransfersForMint(ctx, e.TxSignature, e.WalletAddress, r.usdc)
	if err != nil {
		if errs.CodeOf(err) == errs.CodeNotFound {
			err = errs.Wrap(err, errs.CodeRPCUnavailable, "funding.DepositCandidateResolver.Fetch")
		}
		r.unresolved(ctx, e.WalletAddress, err)
		return DepositCandidateResolution{}, err
	}
	amount, err := transferAmount(transfers, r.usdc)
	if err != nil {
		r.unresolved(ctx, e.WalletAddress, err)
		return DepositCandidateResolution{}, err
	}
	if amount.IsZero() {
		return DepositCandidateResolution{}, nil
	}
	for _, owner := range r.owners {
		ours, err := owner.OwnsSignature(ctx, e.TxSignature)
		if err != nil {
			err = errs.Wrap(err, db.CodeFor(err), "funding.DepositCandidateResolver.Fetch")
			r.unresolved(ctx, e.WalletAddress, err)
			return DepositCandidateResolution{}, err
		}
		if ours {
			return DepositCandidateResolution{Amount: amount, Ours: true}, nil
		}
	}
	return DepositCandidateResolution{Amount: amount}, nil
}

func (r DepositCandidateResolver) Apply(
	ctx context.Context,
	tx db.Tx,
	e events.DepositCandidateSeen,
	resolution DepositCandidateResolution,
	at time.Time,
) error {
	ctx = observability.WithActor(ctx, "system:funding.resolve_deposit_candidate")
	status, reason := candidateStatus(resolution)
	n, err := sqlc.New(tx.Queries()).ResolveDepositCandidate(ctx, sqlc.ResolveDepositCandidateParams{
		Status: status, ResolvedAt: at, TxSignature: string(e.TxSignature), WalletAddress: string(e.WalletAddress),
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	if reason != "" {
		if err := tx.Events.Append(ctx, events.DepositCandidateDismissed{
			V: 1, CandidateID: e.CandidateID, UserID: e.UserID, WalletAddress: e.WalletAddress,
			TxSignature: e.TxSignature, Reason: reason,
		}); err != nil {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) {
			observability.Info(ctx, observability.FundingCandidateDismissed,
				slog.String("reason", reason), slog.String("wallet_address", string(e.WalletAddress)))
		})
		return nil
	}
	_, err = r.credits.Apply(ctx, tx, CreditDeposit{
		ID: r.ids.NewV7(), UserID: ids.UserIDFrom(e.UserID), WalletAddress: e.WalletAddress, TxSignature: e.TxSignature,
		Amount: resolution.Amount, Slot: e.Slot, BlockTime: timeOrZero(e.BlockTime), CreditedAt: at,
	})
	if err != nil {
		return err
	}
	tx.AfterCommit(func(ctx context.Context) {
		observability.Info(
			ctx,
			observability.FundingCandidateResolved,
			slog.String(
				"wallet_address",
				string(e.WalletAddress),
			),
			slog.String("amount_micros", resolution.Amount.String()),
		)
	})
	return nil
}

func (r DepositCandidateResolver) unresolved(ctx context.Context, wallet chain.SolanaAddress, err error) {
	observability.Degraded(ctx, observability.FundingCandidateUnresolved,
		slog.String("wallet_address", string(wallet)), slog.String("code", string(errs.CodeOf(err))))
}

func transferAmount(transfers []solana.Transfer, usdc chain.SolanaAddress) (money.Micros, error) {
	amount := money.Micros{}
	for _, transfer := range transfers {
		if transfer.Mint.Address != usdc || transfer.Net.Decimals() != 6 || transfer.Net.IsZero() {
			continue
		}
		next, err := amount.Add(money.MicrosFromUint64(transfer.Net.Uint64()))
		if err != nil {
			return money.Micros{}, err
		}
		amount = next
	}
	return amount, nil
}

func candidateStatus(resolution DepositCandidateResolution) (string, string) {
	if resolution.Amount.IsZero() {
		return "not_deposit", "not_deposit"
	}
	if resolution.Ours {
		return "ours", "ours"
	}
	return "credited", ""
}

func timeOrZero(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return *value
}
