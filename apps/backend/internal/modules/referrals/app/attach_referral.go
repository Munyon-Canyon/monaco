package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/referrals/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type AttachReferral struct {
	CallerID       ids.UserID
	Code           string
	Source         string
	IdempotencyKey string
}

type AttachReferralDeps struct {
	UoW      *db.UnitOfWork
	Resolver Resolver
	Users    identity.UserReader
	IDs      ids.Generator
	Clock    clock.Clock
}

type AttachReferralHandler struct{ d AttachReferralDeps }

func NewAttachReferralHandler(d AttachReferralDeps) *AttachReferralHandler {
	return &AttachReferralHandler{d: d}
}

func (h *AttachReferralHandler) Handle(ctx context.Context, cmd AttachReferral) error {
	return h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		resolved, err := h.d.Resolver.Resolve(ctx, cmd.Code)
		if err != nil {
			return err
		}
		now := h.d.Clock.Now().UTC()
		if err := h.allowed(ctx, tx, resolved, cmd.CallerID, now); err != nil {
			return err
		}
		id := h.d.IDs.NewV7()
		if err := h.insert(ctx, tx, id, resolved, cmd, now); err != nil {
			return err
		}
		if err := tx.Events.Append(ctx, events.ReferralAttributed{
			V: 1, ReferralID: id, Referrer: resolved.UserID.UUID(), Referee: cmd.CallerID.UUID(),
			CodeKind: string(resolved.CodeKind), Source: cmd.Source, AttributedAt: now,
		}); err != nil {
			return err
		}
		observability.Info(ctx, observability.ReferralsAttributed,
			slog.String("referral_id", id.String()), slog.String("code_kind", string(resolved.CodeKind)),
			slog.String("source", cmd.Source))
		faultpoint.Hit(ctx, faultpoint.BeforeCommit)
		return nil
	})
}

func (h *AttachReferralHandler) allowed(
	ctx context.Context, tx db.Tx, resolved Resolved, callerID ids.UserID, now time.Time,
) error {
	if resolved.UserID == callerID {
		return errs.New(errs.CodeReferralSelf, "referrals.AttachReferral")
	}
	attached, err := sqlc.New(tx.Queries()).ReferralAttached(ctx, callerID.UUID())
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "referrals.AttachReferral")
	}
	if attached {
		return errs.New(errs.CodeReferralAlreadyAttached, "referrals.AttachReferral")
	}
	cards, err := h.d.Users.UsersByID(ctx, []ids.UserID{callerID})
	if err != nil {
		return err
	}
	caller, ok := cards[callerID]
	if !ok {
		return errs.New(errs.CodeInternal, "referrals.AttachReferral")
	}
	if !windowOpen(caller, now) {
		return errs.New(errs.CodeReferralWindowClosed, "referrals.AttachReferral")
	}
	return nil
}

func (h *AttachReferralHandler) insert(
	ctx context.Context, tx db.Tx, id uuid.UUID, resolved Resolved, cmd AttachReferral, now time.Time,
) error {
	_, err := sqlc.New(tx.Queries()).InsertReferral(ctx, sqlc.InsertReferralParams{
		ID: id, ReferrerID: resolved.UserID.UUID(), RefereeID: cmd.CallerID.UUID(),
		Code: string(resolved.Code), CodeKind: string(resolved.CodeKind), Source: cmd.Source, AttributedAt: now,
	})
	return referralInsertError(err)
}

func referralInsertError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return errs.New(errs.CodeReferralAlreadyAttached, "referrals.AttachReferral")
	}
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "referrals.AttachReferral")
	}
	return nil
}

func windowOpen(caller identity.UserCard, now time.Time) bool {
	if !now.Before(caller.CreatedAt.Add(7 * 24 * time.Hour)) {
		return false
	}
	return caller.AuthState != identity.AuthOnboardingCompleted ||
		!caller.AuthStateChangedAt.Add(24*time.Hour).Before(now)
}
