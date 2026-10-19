package app

import (
	"context"

	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const OneMonth = rankingport.Range1M

type PersonReturn = rankingport.PersonReturn

type Returns interface {
	PeopleReturns(
		ctx context.Context, rng rankingport.Range, users []ids.UserID,
	) (map[ids.UserID]PersonReturn, error)
}
