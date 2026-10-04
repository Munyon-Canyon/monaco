package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type OnboardingUsers interface {
	FindByID(ctx context.Context, q sqlc.DBTX, id ids.UserID) (domain.User, error)
	LockByID(ctx context.Context, q sqlc.DBTX, id ids.UserID) (domain.User, error)
	Onboard(ctx context.Context, q sqlc.DBTX, id ids.UserID, sync domain.LinkSync, at time.Time) error
}

type OnboardingDeps struct {
	UoW   *db.UnitOfWork
	Reads sqlc.DBTX
	Users OnboardingUsers
	Privy PrivyUsers
	Clock clock.Clock
	Hints Hints
}

type LinkPhone struct{ UserID ids.UserID }

type LinkSocials struct{ UserID ids.UserID }

type SkipOnboardingStep struct {
	UserID ids.UserID
	Step   domain.OnboardingStep
}

type Onboarding struct{ d OnboardingDeps }

func NewOnboarding(d OnboardingDeps) *Onboarding { return &Onboarding{d: d} }

func (o *Onboarding) LinkPhone(ctx context.Context, cmd LinkPhone) (Me, error) {
	return o.link(ctx, cmd.UserID, domain.PhoneVerified)
}

func (o *Onboarding) LinkSocials(ctx context.Context, cmd LinkSocials) (Me, error) {
	return o.link(ctx, cmd.UserID, domain.XLinked)
}

func (o *Onboarding) Skip(ctx context.Context, cmd SkipOnboardingStep) (Me, error) {
	if _, err := o.withHandle(ctx, cmd.UserID); err != nil {
		return Me{}, err
	}
	switch cmd.Step {
	case domain.StepPhone:
		return o.apply(ctx, cmd.UserID, domain.PhoneSkipped, domain.Links{})
	case domain.StepSocials:
		return GetMe(ctx, o.d.Reads, cmd.UserID)
	}
	return Me{}, errs.New(errs.CodeInvalidInput, "identity.SkipOnboardingStep", slog.String("step", string(cmd.Step)))
}

func (o *Onboarding) link(ctx context.Context, id ids.UserID, kind domain.AuthEventKind) (Me, error) {
	u, err := o.withHandle(ctx, id)
	if err != nil {
		return Me{}, err
	}
	privy, err := o.d.Privy.User(ctx, PrivyUserID(u.PrivyUserID))
	if err != nil {
		return Me{}, err
	}
	return o.apply(ctx, id, kind, privy.Links())
}

func (o *Onboarding) withHandle(ctx context.Context, id ids.UserID) (domain.User, error) {
	u, err := o.d.Users.FindByID(ctx, o.d.Reads, id)
	if err != nil {
		return domain.User{}, err
	}
	return u, domain.HandleSet(u)
}

func (o *Onboarding) apply(
	ctx context.Context, id ids.UserID, kind domain.AuthEventKind, privy domain.Links,
) (Me, error) {
	err := o.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		u, err := o.d.Users.LockByID(ctx, tx.Queries(), id)
		if err != nil {
			return err
		}
		sync, err := domain.Onboard(u, kind, privy)
		if err != nil || sync.Empty() {
			return err
		}
		now := o.d.Clock.Now()
		if err := o.d.Users.Onboard(ctx, tx.Queries(), id, sync, now); err != nil {
			return err
		}
		if err := appendAuthSteps(ctx, tx, id, sync.Steps, now); err != nil {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) {
			o.d.Hints.PublishHint(ctx, "user."+id.String()+".me_changed", nil)
		})
		return nil
	})
	if err != nil {
		return Me{}, err
	}
	return GetMe(ctx, o.d.Reads, id)
}

func appendAuthSteps(ctx context.Context, tx db.Tx, id ids.UserID, steps []domain.AuthStep, at time.Time) error {
	for _, step := range steps {
		err := tx.Events.Append(ctx, events.UserAuthStateChanged{
			V: 1, UserID: id.UUID(), From: string(step.From), To: string(step.To), Cause: string(step.Cause), At: at,
		})
		if err != nil {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) {
			observability.Info(ctx, observability.IdentityOnboardingAdvanced, slog.String("user_id", id.String()),
				slog.String("from", string(step.From)), slog.String("to", string(step.To)),
				slog.String("cause", string(step.Cause)))
		})
	}
	return nil
}
