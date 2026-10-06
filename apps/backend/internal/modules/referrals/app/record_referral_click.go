package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const unknownCodeKind = "unknown"

type RecordReferralClick struct {
	Code string
}

type RecordReferralClickDeps struct {
	Resolver Resolver
	Reads    sqlc.DBTX
	Clock    clock.Clock
}

type RecordReferralClickHandler struct{ d RecordReferralClickDeps }

func NewRecordReferralClickHandler(d RecordReferralClickDeps) *RecordReferralClickHandler {
	return &RecordReferralClickHandler{d: d}
}

func (h *RecordReferralClickHandler) Handle(ctx context.Context, cmd RecordReferralClick) error {
	const op = "referrals.RecordReferralClick"
	resolved, err := h.d.Resolver.Resolve(ctx, cmd.Code)
	if errs.CodeOf(err) == errs.CodeReferralCodeUnknown {
		observability.Info(ctx, observability.ReferralsClick,
			slog.String("code_kind", unknownCodeKind), slog.Bool("known", false))
		return nil
	}
	if err != nil {
		return err
	}
	err = sqlc.New(h.d.Reads).CountReferralClick(ctx, sqlc.CountReferralClickParams{
		Code: string(resolved.Code), Day: h.d.Clock.Now().UTC().Format(time.DateOnly),
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	observability.Info(ctx, observability.ReferralsClick,
		slog.String("code_kind", string(resolved.CodeKind)), slog.Bool("known", true))
	return nil
}
