package domain

import (
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type OnboardingStep string

const (
	StepPhone   OnboardingStep = "phone"
	StepSocials OnboardingStep = "socials"
)

func Onboard(u User, kind AuthEventKind, privy Links) (LinkSync, error) {
	const op = "identity.Onboard"
	f := linkFold{state: u.AuthState, phone: u.Links.Phone != "", x: u.Links.X != nil}
	switch kind {
	case PhoneVerified:
		if privy.Phone == "" {
			return LinkSync{}, errs.New(errs.CodePhoneNotLinked, op)
		}
		if privy.Phone != u.Links.Phone {
			f.sync.Phone = Write[string]{Changed: true, Value: privy.Phone}
		}
		f.phone = true
	case XLinked:
		if privy.X == nil {
			return LinkSync{}, errs.New(errs.CodeXNotLinked, op)
		}
		if u.Links.X == nil || u.Links.X.UserID != privy.X.UserID {
			f.sync.X = Write[*XAccount]{Changed: true, Value: privy.X}
		}
		f.x = true
	case PhoneSkipped:
	case PhoneUnlinked, XUnlinked:
		return LinkSync{}, errs.New(errs.CodeInternal, op, slog.String("event", string(kind)))
	}
	f.move(kind, CauseOnboarding)
	return f.sync, f.err
}
