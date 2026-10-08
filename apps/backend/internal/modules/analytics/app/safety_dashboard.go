package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
)

const bannedStatus = "banned"

type SafetySources struct {
	Events EventCounter
	Cabals cabalport.Dashboard
	Users  identityport.Dashboard
}

type SafetyView struct {
	Window       Window
	Series       []SeriesPoint
	BannedUsers  int64
	BannedCabals int64
}

type Safety struct {
	Read ReadOnly
	Bind func(db dbsqlc.DBTX) SafetySources
}

func safetyReads() []SeriesRead {
	return []SeriesRead{
		{Metric: "cabals_paused", Type: string(events.TypeCabalPaused), GroupBy: "reason"},
		{Metric: "external_deposits_detected", Type: string(events.TypeCabalExternalDepositDetected)},
		{Metric: "external_deposits_bounced", Type: string(events.TypeCabalExternalDepositBounced)},
		{Metric: "admin_actions", Type: string(events.TypeAdminAction), GroupBy: "action"},
	}
}

func (s Safety) Dashboard(ctx context.Context, w Window) (SafetyView, error) {
	view := SafetyView{Window: w}
	err := s.Read(ctx, func(ctx context.Context, db dbsqlc.DBTX) error {
		src := s.Bind(db)
		var states []identityport.StatusCount
		steps := []func(context.Context) error{
			func(ctx context.Context) (err error) {
				view.Series, err = ReadSeries(ctx, src.Events, w, safetyReads())
				return err
			},
			func(ctx context.Context) error {
				counts, err := src.Cabals.Counts(ctx)
				view.BannedCabals = counts.Banned
				return err
			},
			func(ctx context.Context) (err error) {
				states, err = src.Users.StatusCounts(ctx)
				return err
			},
		}
		for _, step := range steps {
			if err := step(ctx); err != nil {
				return err
			}
		}
		for _, state := range states {
			if state.AccountStatus == bannedStatus {
				view.BannedUsers += state.Users
			}
		}
		return nil
	})
	if err != nil {
		return SafetyView{}, err
	}
	return view, nil
}
