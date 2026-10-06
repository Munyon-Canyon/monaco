package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type SnapshotWriter struct {
	uow *db.UnitOfWork
	ids ids.Generator
}

func NewSnapshotWriter(uow *db.UnitOfWork, ids ids.Generator) SnapshotWriter {
	return SnapshotWriter{uow: uow, ids: ids}
}

func (w SnapshotWriter) Write(
	ctx context.Context,
	valuation Valuation,
	startedAt, finishedAt time.Time,
) (uuid.UUID, error) {
	runID := w.ids.NewV7()
	err := w.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return w.persist(ctx, tx, runID, valuation, startedAt, finishedAt)
	})
	return runID, err
}

func (w SnapshotWriter) persist(
	ctx context.Context,
	tx db.Tx,
	runID uuid.UUID,
	valuation Valuation,
	startedAt, finishedAt time.Time,
) error {
	return w.persistQueries(ctx, sqlc.New(tx.Queries()), tx.Events, runID, valuation, startedAt, finishedAt)
}

type discardEvents struct{}

func (discardEvents) Append(context.Context, events.Event) error { return nil }

func ReplaySnapshot(
	ctx context.Context,
	tx db.Tx,
	written events.RankingSnapshotWritten,
	valuation Valuation,
) error {
	return SnapshotWriter{}.persistQueries(
		ctx, sqlc.New(tx.Queries()), discardEvents{}, written.RunID, valuation, written.AsOf, written.ComputedAt,
	)
}

type rankingQueries interface {
	snapshotQueries
	DeleteAllLeaderboardEntries(context.Context) error
	InsertLeaderboardEntries(context.Context, []byte) error
	DeleteRankingTriggersThrough(context.Context, time.Time) error
	PreviousReader
}

type eventAppender interface {
	Append(context.Context, events.Event) error
}

func (w SnapshotWriter) persistQueries(
	ctx context.Context,
	q rankingQueries,
	event eventAppender,
	runID uuid.UUID,
	valuation Valuation,
	startedAt, finishedAt time.Time,
) error {
	if err := validate(valuation); err != nil {
		return err
	}
	if err := keepsFlagged(ctx, q, valuation); err != nil {
		return err
	}
	if err := q.DeleteAllLeaderboardEntries(ctx); err != nil {
		return err
	}
	encoded, err := json.Marshal(valuation.Entries)
	if err != nil {
		return errs.New(errs.CodeInvalidInput, "ranking.SnapshotWriter.Write")
	}
	if err := q.InsertLeaderboardEntries(ctx, encoded); err != nil {
		return err
	}
	if err := w.snapshots(ctx, q, valuation); err != nil {
		return err
	}
	if err := w.run(ctx, q, runID, valuation, startedAt, finishedAt); err != nil {
		return err
	}
	if err := q.DeleteRankingTriggersThrough(ctx, startedAt); err != nil {
		return err
	}
	if err := event.Append(
		ctx,
		events.RankingSnapshotWritten{
			V:              1,
			RunID:          runID,
			AsOf:           valuation.AsOf,
			PricesAsOf:     valuation.PricesAsOf,
			ComputedAt:     finishedAt,
			RowsWritten:    len(valuation.Entries),
			CabalsExcluded: valuation.Excluded,
		},
	); err != nil {
		return err
	}
	faultpoint.Hit(ctx, faultpoint.BeforeCommit)
	return nil
}

type snapshotQueries interface {
	InsertCabalValueSnapshot(context.Context, sqlc.InsertCabalValueSnapshotParams) error
	InsertLeaderboardRun(context.Context, sqlc.InsertLeaderboardRunParams) error
}

func (w SnapshotWriter) snapshots(ctx context.Context, q snapshotQueries, valuation Valuation) error {
	for _, cabal := range valuation.Cabals {
		amounts, err := int64s(cabal)
		if err != nil {
			return err
		}
		if err := q.InsertCabalValueSnapshot(
			ctx,
			sqlc.InsertCabalValueSnapshotParams{
				CabalID:           cabal.CabalID.UUID(),
				At:                valuation.AsOf,
				ValueMicros:       amounts[0],
				NavPerShareMicros: amounts[1],
				TotalShares:       amounts[2],
			},
		); err != nil {
			return err
		}
	}
	return nil
}

func (w SnapshotWriter) run(
	ctx context.Context,
	q snapshotQueries,
	runID uuid.UUID,
	valuation Valuation,
	startedAt, finishedAt time.Time,
) error {
	counts, err := int32s(valuation.Excluded, len(valuation.Entries))
	if err != nil {
		return err
	}
	return q.InsertLeaderboardRun(
		ctx,
		sqlc.InsertLeaderboardRunParams{
			RunID:          runID,
			AsOf:           valuation.AsOf,
			PricesAsOf:     valuation.PricesAsOf,
			StartedAt:      startedAt,
			FinishedAt:     finishedAt,
			RowsWritten:    counts[1],
			CabalsExcluded: counts[0],
		},
	)
}

func int32s(ns ...int) ([]int32, error) {
	out := make([]int32, len(ns))
	for i, n := range ns {
		if n < 0 || n > math.MaxInt32 {
			return nil, errs.New(errs.CodeInvalidInput, "ranking.SnapshotWriter.Write")
		}
		out[i] = int32(n)
	}
	return out, nil
}

func int64s(cabal CabalValue) ([3]int64, error) {
	var out [3]int64
	for i, n := range [3]uint64{cabal.Value.Uint64(), cabal.NavPerShare.Uint64(), cabal.TotalShares.Uint64()} {
		if n > math.MaxInt64 {
			return out, errs.New(errs.CodeInvalidInput, "ranking.SnapshotWriter.Write")
		}
		out[i] = int64(n)
	}
	return out, nil
}

func keepsFlagged(ctx context.Context, q PreviousReader, valuation Valuation) error {
	flagged := make([]ids.CabalID, len(valuation.Flagged))
	for i, cabal := range valuation.Flagged {
		flagged[i] = cabal.CabalID
	}
	previous, err := PreviousEntries(ctx, q, flagged)
	if err != nil {
		return err
	}
	kept := make(map[[3]string]bool, len(valuation.Entries))
	for _, entry := range valuation.Entries {
		kept[[3]string{entry.Board, entry.SubjectID.String(), entry.Range}] = true
	}
	for _, entry := range previous {
		if !kept[[3]string{entry.Board, entry.SubjectID.String(), entry.Range}] {
			return errs.New(errs.CodeInvalidInput, "ranking.SnapshotWriter.Write", slog.String("board", entry.Board))
		}
	}
	return nil
}

func validate(valuation Valuation) error {
	unbuilt := valuation.Entries == nil || (len(valuation.Cabals) > 0 && len(valuation.Entries) == 0)
	allExcluded := valuation.Excluded > 0 && len(valuation.Cabals) == 0 && len(valuation.Flagged) == 0
	if unbuilt || allExcluded {
		return errs.New(errs.CodeInvalidInput, "ranking.SnapshotWriter.Write")
	}
	return nil
}
