package marketing

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/providers/x"
)

const coldStart = time.Hour

type Source interface {
	Fetch(ctx context.Context, rule Rule, cursor string) (x.Page, error)
}

type Searcher struct {
	Client *x.Client
	Config Config
	Clock  clock.Clock
}

func (s Searcher) Fetch(ctx context.Context, r Rule, cursor string) (x.Page, error) {
	return s.Client.Search(ctx, x.SearchRequest{
		Query: s.Config.Query(r), SinceID: cursor, Start: s.Clock.Now().Add(-coldStart),
	})
}
