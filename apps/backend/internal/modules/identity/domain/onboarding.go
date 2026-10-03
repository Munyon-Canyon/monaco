package domain

import (
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func Onboard(u User, kind AuthEventKind, privy Links) (LinkSync, error) {
	const op = "identity.Onboard"
	f := linkFold{state: u.AuthState, phone: u.Links.Phone != "", x: u.Links.X != nil}
	if kind != PhoneVerified {
		return LinkSync{}, errs.New(errs.CodeInternal, op, slog.String("event", string(kind)))
	}
	if privy.Phone == "" {
		return LinkSync{}, errs.New(errs.CodePhoneNotLinked, op)
	}
	if privy.Phone != u.Links.Phone {
		f.sync.Phone = Write[string]{Changed: true, Value: privy.Phone}
	}
	f.phone = true
	f.move(kind, CauseOnboarding)
	return f.sync, f.err
}
