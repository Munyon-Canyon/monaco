package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
)

type SocialSources struct {
	Events EventCounter
	Cabals cabalport.Dashboard
	Users  identityport.Dashboard
}

type SocialView struct {
	Window Window
	Series []SeriesPoint
	Cabals cabalport.Counts
	States []identityport.StatusCount
	Users  int64
}

type Social struct {
	Read ReadOnly
	Bind func(db dbsqlc.DBTX) SocialSources
}

func socialReads() []SeriesRead {
	return []SeriesRead{
		{Metric: "users_created", Type: string(events.TypeUserCreated)},
		{Metric: "follows_created", Type: string(events.TypeFollowCreated), GroupBy: "source"},
		{Metric: "follows_removed", Type: string(events.TypeFollowRemoved)},
		{Metric: "comments_created", Type: string(events.TypeCommentCreated)},
		{Metric: "replies_created", Type: string(events.TypeCommentCreated), Present: "parent_comment_id"},
	}
}

func (s Social) Dashboard(ctx context.Context, w Window) (SocialView, error) {
	view := SocialView{Window: w}
	err := s.Read(ctx, func(ctx context.Context, db dbsqlc.DBTX) error {
		src := s.Bind(db)
		steps := []func(context.Context) error{
			func(ctx context.Context) (err error) {
				view.Series, err = ReadSeries(ctx, src.Events, w, socialReads())
				return err
			},
			func(ctx context.Context) (err error) {
				view.Cabals, err = src.Cabals.Counts(ctx)
				return err
			},
			func(ctx context.Context) (err error) {
				view.States, err = src.Users.StatusCounts(ctx)
				return err
			},
		}
		for _, step := range steps {
			if err := step(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return SocialView{}, err
	}
	for _, state := range view.States {
		view.Users += state.Users
	}
	return view, nil
}
