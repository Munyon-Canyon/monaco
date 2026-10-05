package app

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	FundSettleInterval = 5 * time.Second
	FundSendWindow     = 2 * time.Minute
	fundBatch          = 100
)

type FundChain interface {
	SignatureStatuses(ctx context.Context, sigs []chain.Signature) ([]solana.Status, error)
}

type FundBroadcaster interface {
	Broadcast(ctx context.Context, tx relayer.SignedTx) error
}

type HintPublisher interface {
	PublishHint(ctx context.Context, key string, payload []byte)
}

type FundSettlerDeps struct {
	Reads     sqlc.DBTX
	UoW       *db.UnitOfWork
	IDs       ids.Generator
	Clock     clock.Clock
	Chain     FundChain
	Transfers FundBroadcaster
	Pot       FundPot
	Ledger    Ledger
	USDC      domain.Asset
	Hints     HintPublisher
}

type FundSettler struct{ d FundSettlerDeps }

func NewFundSettler(d FundSettlerDeps) *FundSettler { return &FundSettler{d: d} }

type openFund = sqlc.ListOpenFundTransfersRow

type FundTick struct {
	Scanned, Expired, Changed, Minted int
}

func (s *FundSettler) Tick(ctx context.Context) (FundTick, error) {
	q := sqlc.New(s.d.Reads)
	expired, expireErr := q.ExpireCreatedFundTransfers(ctx, s.d.Clock.Now().Add(-FundSendWindow))
	rows, listErr := q.ListOpenFundTransfers(ctx, fundBatch)
	if err := errors.Join(expireErr, listErr); err != nil {
		return FundTick{}, err
	}
	tick := FundTick{Scanned: len(rows), Expired: len(expired)}
	var submitted, landed []openFund
	for _, r := range rows {
		if r.Status == string(domain.FundSubmitted) {
			submitted = append(submitted, r)
		} else {
			landed = append(landed, r)
		}
	}
	confirmed, changed, err := s.confirm(ctx, submitted)
	tick.Changed = changed
	if err != nil {
		return tick, err
	}
	for _, r := range append(landed, confirmed...) {
		ok, err := s.mint(ctx, r)
		if err != nil {
			return tick, err
		}
		if ok {
			tick.Minted++
		}
	}
	return tick, nil
}

func (s *FundSettler) confirm(ctx context.Context, rows []openFund) ([]openFund, int, error) {
	if len(rows) == 0 {
		return nil, 0, nil
	}
	sigs := make([]chain.Signature, len(rows))
	for i, r := range rows {
		sigs[i] = chain.Signature(r.TxSignature)
	}
	statuses, err := s.d.Chain.SignatureStatuses(ctx, sigs)
	if err != nil {
		return nil, 0, err
	}
	var landed []openFund
	changed := 0
	for i, st := range statuses {
		r := rows[i]
		switch {
		case st.Failed:
			err = s.fail(ctx, r, errs.CodeFundRejected)
		case st.State == solana.StateFinalized:
			var moved bool
			if moved, err = s.land(ctx, r); moved {
				landed = append(landed, r)
			}
		case st.BlockhashExpired(big.NewInt(r.LastValidBlockHeight).Uint64()):
			err = s.fail(ctx, r, errs.CodeFundExpired)
		case st.State == solana.StateNotFound:
			s.rebroadcast(ctx, r)
			continue
		default:
			continue
		}
		if err != nil {
			return landed, changed, err
		}
		changed++
	}
	return landed, changed, nil
}

func (s *FundSettler) land(ctx context.Context, r openFund) (bool, error) {
	var moved bool
	err := s.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		n, err := sqlc.New(tx.Queries()).LandFundTransfer(ctx, sqlc.LandFundTransferParams{
			ID: r.ID, LandedAt: s.d.Clock.Now(),
		})
		moved = n == 1
		return err
	})
	return moved, err
}

func (s *FundSettler) fail(ctx context.Context, r openFund, code errs.Code) error {
	amount, err := money.ParseMicros(r.AmountMicros)
	if err != nil {
		return err
	}
	user := ids.UserIDFrom(r.UserID)
	err = s.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		n, err := sqlc.New(tx.Queries()).FailFundTransfer(ctx, sqlc.FailFundTransferParams{
			ID: r.ID, FailCode: string(code),
		})
		if err != nil || n == 0 {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) {
			s.d.Hints.PublishHint(ctx, events.UserBalanceChangedHint(user), nil)
		})
		return tx.Events.Append(ctx, events.FundFailed{
			V: 1, TransferID: r.ID, CabalID: r.CabalID, UserID: r.UserID, AmountMicros: amount, Code: string(code),
		})
	})
	if err != nil {
		return err
	}
	observability.Info(ctx, observability.TreasuryFundFailed, slog.String("transfer_id", r.ID.String()),
		slog.String("code", string(code)), slog.String("status_before", r.Status),
		slog.String("status_after", string(domain.FundFailed)))
	return nil
}

func (s *FundSettler) rebroadcast(ctx context.Context, r openFund) {
	err := s.d.Transfers.Broadcast(ctx, relayer.SignedTx{Bytes: r.SignedTx, Signature: chain.Signature(r.TxSignature)})
	if err != nil {
		observability.Info(ctx, observability.TreasuryFundBroadcastFailed, slog.String("transfer_id", r.ID.String()),
			slog.String("code", string(errs.CodeOf(err))))
	}
}
