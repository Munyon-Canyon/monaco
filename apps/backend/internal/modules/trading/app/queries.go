package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type SwapView struct {
	ID          ids.SwapID
	CabalID     ids.CabalID
	Source      domain.Source
	Action      domain.Action
	Symbol      string
	InAmount    uint64
	OutAmount   uint64
	Status      domain.Status
	FailureCode domain.FailureCode
	TxSignature chain.Signature
	CreatedAt   time.Time
	ConfirmedAt time.Time
	Retryable   bool
}

type Queries struct {
	q *sqlc.Queries
}

func NewQueries(db sqlc.DBTX) Queries { return Queries{q: sqlc.New(db)} }

func (s Queries) Swap(ctx context.Context, id ids.SwapID) (SwapView, error) {
	row, err := s.q.SwapByID(ctx, id.UUID())
	return found("trading.Swap", row, err)
}

func (s Queries) SwapBySignature(ctx context.Context, sig chain.Signature) (SwapView, error) {
	row, err := s.q.SwapBySignature(ctx, string(sig))
	return found("trading.SwapBySignature", row, err)
}

func (s Queries) LatestBySource(ctx context.Context, src domain.Source) (SwapView, bool, error) {
	const op = "trading.LatestBySource"
	row, err := s.q.LatestBySource(ctx, sqlc.LatestBySourceParams{SourceKind: string(src.Kind), SourceID: src.ID})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return SwapView{}, false, nil
	case err != nil:
		return SwapView{}, false, errs.Wrap(err, errs.CodeInternal, op)
	}
	v, err := view(op, row)
	return v, err == nil, err
}

func (s Queries) HasLiveSwap(ctx context.Context, src domain.Source) (bool, error) {
	live, err := s.q.HasLiveSwap(ctx, sqlc.HasLiveSwapParams{SourceKind: string(src.Kind), SourceID: src.ID})
	if err != nil {
		return false, errs.Wrap(err, errs.CodeInternal, "trading.HasLiveSwap")
	}
	return live, nil
}

func (s Queries) OwnsSignature(ctx context.Context, sig chain.Signature) (bool, error) {
	owns, err := s.q.OwnsSignature(ctx, string(sig))
	if err != nil {
		return false, errs.Wrap(err, errs.CodeInternal, "trading.OwnsSignature")
	}
	return owns, nil
}

func found(op string, row sqlc.SwapView, err error) (SwapView, error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return SwapView{}, errs.New(errs.CodeSwapNotFound, op)
	case err != nil:
		return SwapView{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return view(op, row)
}

func view(op string, r sqlc.SwapView) (SwapView, error) {
	kind, kindErr := domain.ParseSourceKind(r.SourceKind)
	action, actionErr := domain.ParseAction(r.Action)
	status, statusErr := domain.ParseStatus(r.Status)
	failure, failureErr := optionalFailure(r.FailureCode.String, r.FailureCode.Valid)
	in, inErr := domain.ParseAmount(r.InAmount)
	out, outErr := optionalAmount(r.OutAmount.Int64, r.OutAmount.Valid)
	requiredErr := requireColumns(op, status, r)
	if err := errors.Join(kindErr, actionErr, statusErr, failureErr, inErr, outErr, requiredErr); err != nil {
		return SwapView{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	return SwapView{
		ID: ids.SwapIDFrom(
			r.ID,
		),
		CabalID:     ids.CabalIDFrom(r.CabalID),
		Source:      domain.Source{Kind: kind, ID: r.SourceID},
		Action:      action,
		Symbol:      r.Symbol,
		InAmount:    in,
		OutAmount:   out,
		Status:      status,
		FailureCode: failure,
		TxSignature: chain.Signature(r.TxSignature.String),
		CreatedAt:   r.CreatedAt,
		ConfirmedAt: r.ConfirmedAt.Time,
		Retryable:   r.Retryable.Bool,
	}, nil
}

func requireColumns(op string, status domain.Status, r sqlc.SwapView) error {
	var missing []string
	if (status == domain.StatusSubmitted || status == domain.StatusConfirmed) && !r.TxSignature.Valid {
		missing = append(missing, "tx_signature")
	}
	if status == domain.StatusConfirmed && !r.OutAmount.Valid {
		missing = append(missing, "out_amount")
	}
	if status == domain.StatusConfirmed && !r.ConfirmedAt.Valid {
		missing = append(missing, "confirmed_at")
	}
	if status == domain.StatusFailed && !r.FailureCode.Valid {
		missing = append(missing, "failure_code")
	}
	if len(missing) == 0 {
		return nil
	}
	return errs.New(errs.CodeDecodeFailed, op, slog.String("status", string(status)),
		slog.String("null_columns", strings.Join(missing, ",")))
}

func optionalFailure(raw string, ok bool) (domain.FailureCode, error) {
	if !ok {
		return "", nil
	}
	return domain.ParseFailureCode(raw)
}

func optionalAmount(v int64, ok bool) (uint64, error) {
	if !ok {
		return 0, nil
	}
	return domain.ParseAmount(v)
}
