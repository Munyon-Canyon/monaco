package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type CabalLeaders struct{}

var _ port.CabalLeaders = CabalLeaders{}

func (CabalLeaders) LeadsCabalWithMembers(ctx context.Context, q dbsqlc.DBTX, user ids.UserID) (bool, error) {
	rows, err := sqlc.New(q).LockCreatedCabalMemberCounts(ctx, user.UUID())
	if err != nil {
		return false, errs.Wrap(err, errs.CodeInternal, "cabal.CabalLeaders.LeadsCabalWithMembers")
	}
	for _, row := range rows {
		if row.Members > 1 {
			return true, nil
		}
	}
	return false, nil
}
