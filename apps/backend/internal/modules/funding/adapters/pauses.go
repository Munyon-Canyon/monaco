package adapters

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Pauses struct{ q *sqlc.Queries }

var _ port.Pauses = Pauses{}

func NewPauses(db sqlc.DBTX) Pauses { return Pauses{q: sqlc.New(db)} }

func (p Pauses) IsPaused(ctx context.Context, cabalID ids.CabalID) (port.Pause, error) {
	rows, err := p.q.OpenPausesFor(ctx, cabalID.UUID())
	if err != nil {
		return port.Pause{}, errs.Wrap(err, errs.CodeInternal, "funding.Pauses.IsPaused")
	}
	open := make([]domain.Pause, len(rows))
	for i, r := range rows {
		open[i] = pauseOf(r.ID, r.CabalID, r.Reason, r.CreatedAt)
	}
	paused, reasons := domain.IsPaused(open, cabalID)
	if !paused {
		return port.Pause{}, nil
	}
	return port.Pause{Paused: true, Reasons: reasons, Since: rows[0].CreatedAt}, nil
}

func (p Pauses) PausedCabals(ctx context.Context) (port.PausedSet, error) {
	rows, err := p.q.OpenPauses(ctx)
	if err != nil {
		return port.PausedSet{}, errs.Wrap(err, errs.CodeInternal, "funding.Pauses.PausedCabals")
	}
	set := port.PausedSet{Cabals: map[ids.CabalID][]domain.PauseReason{}}
	for _, r := range rows {
		reason := domain.PauseReason(r.Reason)
		if r.CabalID == uuid.Nil {
			set.Global = true
			continue
		}
		id := ids.CabalIDFrom(r.CabalID)
		if !slices.Contains(set.Cabals[id], reason) {
			set.Cabals[id] = append(set.Cabals[id], reason)
		}
	}
	return set, nil
}

func pauseOf(id, cabalID uuid.UUID, reason string, createdAt time.Time) domain.Pause {
	p := domain.Pause{ID: id, Reason: domain.PauseReason(reason), CreatedAt: createdAt}
	if cabalID != uuid.Nil {
		cabal := ids.CabalIDFrom(cabalID)
		p.CabalID = &cabal
	}
	return p
}
