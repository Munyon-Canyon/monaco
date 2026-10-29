package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	LedgerBatch    = 200
	LedgerInterval = 15 * time.Minute
)

type LedgerQueries interface {
	PendingLedgerUsers(context.Context, sqlc.PendingLedgerUsersParams) ([]uuid.UUID, error)
	ResolvedVotersBetween(context.Context, sqlc.ResolvedVotersBetweenParams) ([]uuid.UUID, error)
}

type LedgerBuild func(context.Context, LedgerInputs, ids.UserID, time.Time) (domain.Ledger, bool, error)

type LedgerReport struct{ Built, Skipped, Failed int }

type BuildLedgers struct {
	Queries LedgerQueries
	UoW     *db.UnitOfWork
	Inputs  func() LedgerInputs
	Cabals  Cabals
	Clock   clock.Clock
	Build   LedgerBuild
	Limit   int32
}

func NewBuildLedgers(
	queries LedgerQueries, uow *db.UnitOfWork, inputs func() LedgerInputs, cabals Cabals, clk clock.Clock,
) BuildLedgers {
	return BuildLedgers{Queries: queries, UoW: uow, Inputs: inputs, Cabals: cabals, Clock: clk, Build: BuildLedger}
}

func LedgerDelivery(now time.Time) time.Time {
	at := now.UTC()
	day := time.Date(at.Year(), at.Month(), at.Day(), 17, 0, 0, 0, time.UTC)
	day = day.AddDate(0, 0, -int(day.Weekday()))
	if day.After(at) {
		day = day.AddDate(0, 0, -7)
	}
	return day
}

func (b BuildLedgers) Run(ctx context.Context) (LedgerReport, error) {
	const op = "ranking.BuildLedgers.Run"
	delivery := LedgerDelivery(b.Clock.Now())
	week, _, _, start, end := domain.LedgerWeek(delivery)
	candidates, err := b.candidates(ctx, start, end)
	if err != nil {
		return LedgerReport{}, err
	}
	pending, err := b.Queries.PendingLedgerUsers(ctx, sqlc.PendingLedgerUsersParams{
		CandidateIds: candidates, Week: week, BatchLimit: b.limit(),
	})
	if err != nil {
		return LedgerReport{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	if len(pending) == 0 {
		return LedgerReport{}, nil
	}
	inputs := b.Inputs()
	if inputs.Window, err = NewLedgerWindow(ctx, inputs.Ports, start, end); err != nil {
		return LedgerReport{}, err
	}
	var report LedgerReport
	var failures []error
	for _, id := range pending {
		user := ids.UserIDFrom(id)
		built, err := b.one(ctx, inputs, user, delivery)
		switch {
		case err != nil:
			report.Failed++
			failures = append(failures, err)
		case built:
			report.Built++
		default:
			report.Skipped++
		}
	}
	return report, errors.Join(failures...)
}

func (b BuildLedgers) limit() int32 {
	if b.Limit > 0 {
		return b.Limit
	}
	return LedgerBatch
}

func (b BuildLedgers) one(ctx context.Context, inputs LedgerInputs, user ids.UserID, delivery time.Time) (bool, error) {
	ledger, ok, err := b.Build(ctx, inputs, user, delivery)
	if err != nil {
		return false, err
	}
	if !ok {
		week, _, _, _, _ := domain.LedgerWeek(delivery)
		return false, b.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
			return StoreSkip(ctx, tx, user, week)
		})
	}
	var inserted bool
	err = b.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		if inserted, err = StoreLedger(ctx, tx, user, ledger, b.Clock.Now()); err != nil || !inserted {
			return err
		}
		ready := events.RankingLedgerReady{V: 1, UserID: user.UUID(), Week: ledger.Week}
		if err := tx.Events.Append(ctx, ready); err != nil {
			return err
		}
		faultpoint.Hit(ctx, faultpoint.BeforeCommit)
		return nil
	})
	return err == nil && inserted, err
}

func (b BuildLedgers) candidates(ctx context.Context, start, end time.Time) ([]uuid.UUID, error) {
	const op = "ranking.BuildLedgers.candidates"
	cabals, err := b.Cabals.AllCabals(ctx)
	if err != nil {
		return nil, err
	}
	cabalIDs := make([]ids.CabalID, len(cabals))
	for i, c := range cabals {
		cabalIDs[i] = c.ID
	}
	members, err := b.Cabals.MembersOf(ctx, cabalIDs)
	if err != nil {
		return nil, err
	}
	voters, err := b.Queries.ResolvedVotersBetween(
		ctx, sqlc.ResolvedVotersBetweenParams{WindowStart: start, WindowEnd: end},
	)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	out := slices.Clone(voters)
	for _, list := range members {
		for _, m := range list {
			out = append(out, m.UserID.UUID())
		}
	}
	slices.SortFunc(out, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	return slices.Compact(out), nil
}

type LedgerPoller struct {
	Builder BuildLedgers
	Every   time.Duration
}

func (LedgerPoller) Name() string { return "ranking.weekly_ledger" }

func (p LedgerPoller) Interval() time.Duration {
	if p.Every > 0 {
		return p.Every
	}
	return LedgerInterval
}

func (p LedgerPoller) Tick(ctx context.Context) (poller.Report, error) {
	got, err := p.Builder.Run(ctx)
	return poller.Report{
		Scanned: got.Built + got.Skipped + got.Failed, Changed: got.Built,
		Attrs: []slog.Attr{slog.Int("skipped", got.Skipped), slog.Int("failed", got.Failed)},
	}, err
}
