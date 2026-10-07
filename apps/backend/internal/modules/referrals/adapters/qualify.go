package adapters

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type Qualify struct {
	Users app.UserReader
}

func (h Qualify) Handle(ctx context.Context, tx db.Tx, e events.Funded, at time.Time) error {
	if domain.SkipReason(e.AmountMicros, true) == domain.SkipBelowMinimum {
		observability.Debug(ctx, observability.ReferralsQualifySkipped,
			slog.String("reason", domain.SkipBelowMinimum))
		return nil
	}
	q := sqlc.New(tx.Queries())
	referral, err := q.ReferralToQualify(ctx, e.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "referrals.Qualify")
	}
	user := ids.UserIDFrom(e.UserID)
	cards, err := h.Users.UsersByID(ctx, []ids.UserID{user})
	if err != nil {
		return err
	}
	if card, ok := cards[user]; !ok || !card.PhoneVerified {
		observability.Debug(ctx, observability.ReferralsQualifySkipped,
			slog.String("reason", domain.SkipPhoneUnverified))
		return nil
	}
	qualified, err := q.QualifyReferral(ctx, sqlc.QualifyReferralParams{ID: referral.ID, QualifiedAt: at})
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "referrals.Qualify")
	}
	if qualified == 0 {
		return nil
	}
	if err := tx.Events.Append(ctx, events.ReferralQualified{
		V: 1, ReferralID: referral.ID, Referrer: referral.ReferrerID, Referee: e.UserID,
		CabalID: e.CabalID, AmountMicros: e.AmountMicros, QualifiedAt: at,
	}); err != nil {
		return err
	}
	observability.Info(ctx, observability.ReferralsQualified, slog.String("referral_id", referral.ID.String()))
	return nil
}
