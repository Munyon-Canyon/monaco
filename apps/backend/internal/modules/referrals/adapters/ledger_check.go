package adapters

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/sqlc"
)

func CheckReferrals(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	q := sqlc.New(pool)
	many, manyErr := q.RefereesWithManyReferrals(ctx)
	unproven, unprovenErr := q.QualifiedReferralsWithoutProof(ctx)
	columns, columnsErr := q.ReferralClickColumns(ctx)
	if err := errors.Join(manyErr, unprovenErr, columnsErr); err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "referrals.CheckReferrals")
	}
	var out []string
	for _, r := range many {
		out = append(out, "referee "+r.RefereeID.String()+" has more than one referral")
	}
	for _, r := range unproven {
		if !r.HasQualifiedAt {
			out = append(out, "qualified referral "+r.ID.String()+" has no qualified_at")
		}
		if r.Events != 1 {
			out = append(out, "qualified referral "+r.ID.String()+" has "+strconv.FormatInt(r.Events, 10)+
				" referral.qualified events, want 1")
		}
	}
	if !slices.Equal(columns, []string{"clicks", "code", "day"}) {
		out = append(out, "referral_clicks has columns "+strings.Join(columns, ", ")+", want code, day, clicks only")
	}
	return out, nil
}
