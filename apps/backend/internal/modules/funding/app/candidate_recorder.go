package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type DepositCandidate struct {
	Signature chain.Signature
	Wallet    chain.SolanaAddress
	UserID    ids.UserID
	Slot      int64
	BlockTime time.Time
	Source    string
}

type CandidateRecorder struct {
	ids ids.Generator
	now func() time.Time
}

func NewCandidateRecorder(g ids.Generator, now func() time.Time) CandidateRecorder {
	return CandidateRecorder{ids: g, now: now}
}

func (r CandidateRecorder) Record(ctx context.Context, tx db.Tx, candidates []DepositCandidate) (int, error) {
	q := sqlc.New(tx.Queries())
	inserted := 0
	for _, candidate := range candidates {
		n, err := q.InsertDepositCandidate(ctx, sqlc.InsertDepositCandidateParams{
			TxSignature: string(
				candidate.Signature,
			),
			WalletAddress: string(candidate.Wallet),
			UserID:        candidate.UserID.UUID(),
			Slot:          candidate.Slot,
			BlockTime:     candidate.BlockTime,
			Source:        candidate.Source,
			SeenAt:        r.now(),
		})
		if err != nil {
			return 0, err
		}
		if n == 0 {
			continue
		}
		if err := tx.Events.Append(ctx, events.DepositCandidateSeen{
			V:             1,
			CandidateID:   r.ids.NewV7(),
			UserID:        candidate.UserID.UUID(),
			WalletAddress: candidate.Wallet,
			TxSignature:   candidate.Signature,
			Slot:          candidate.Slot,
			BlockTime:     optionalTime(candidate.BlockTime),
			Source:        candidate.Source,
		}); err != nil {
			return 0, err
		}
		observability.Debug(ctx, observability.FundingCandidateRecorded,
			slog.String("wallet_address", string(candidate.Wallet)))
		inserted++
	}
	return inserted, nil
}
