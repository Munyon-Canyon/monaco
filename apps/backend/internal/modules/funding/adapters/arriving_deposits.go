package adapters

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type ArrivingDeposits struct{}

var _ port.DepositsArriving = ArrivingDeposits{}

func (ArrivingDeposits) DepositArriving(
	ctx context.Context, q dbsqlc.DBTX, user ids.UserID, at time.Time,
) (bool, error) {
	arriving, err := sqlc.New(q).UserHasArrivingDeposit(ctx, sqlc.UserHasArrivingDepositParams{
		Member: user.UUID(), Since: at.Add(-app.OnrampArrivalWindow), OpenedSince: at.Add(-app.OnrampOpenedTTL),
	})
	if err != nil {
		return false, errs.Wrap(err, errs.CodeInternal, "funding.ArrivingDeposits.DepositArriving")
	}
	return arriving, nil
}
