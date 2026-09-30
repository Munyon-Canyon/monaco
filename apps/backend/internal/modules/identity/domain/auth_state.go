package domain

import (
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type AuthState string

const (
	AuthCreated             AuthState = "CREATED"
	AuthAwaitingPhone       AuthState = "AWAITING_PHONE"
	AuthAwaitingSocials     AuthState = "AWAITING_SOCIALS"
	AuthOnboardingCompleted AuthState = "ONBOARDING_COMPLETED"
)

type AuthEventKind string

const (
	PhoneVerified AuthEventKind = "phone_verified"
	PhoneSkipped  AuthEventKind = "phone_skipped"
	XLinked       AuthEventKind = "x_linked"
	PhoneUnlinked AuthEventKind = "phone_unlinked"
	XUnlinked     AuthEventKind = "x_unlinked"
)

type AuthEvent struct {
	Kind     AuthEventKind
	HasPhone bool
	HasX     bool
}

func (s AuthState) known() bool {
	switch s {
	case AuthCreated, AuthAwaitingPhone, AuthAwaitingSocials, AuthOnboardingCompleted:
		return true
	}
	return false
}

func (k AuthEventKind) known() bool {
	switch k {
	case PhoneVerified, PhoneSkipped, XLinked, PhoneUnlinked, XUnlinked:
		return true
	}
	return false
}

type authStep struct {
	from AuthState
	kind AuthEventKind
}

func authSteps() map[authStep]func(AuthEvent) AuthState {
	return map[authStep]func(AuthEvent) AuthState{
		{AuthCreated, PhoneVerified}:             afterPhone,
		{AuthCreated, PhoneSkipped}:              to(AuthAwaitingPhone),
		{AuthCreated, XLinked}:                   xBeforePhone,
		{AuthAwaitingPhone, PhoneVerified}:       afterPhone,
		{AuthAwaitingSocials, XLinked}:           to(AuthOnboardingCompleted),
		{AuthAwaitingSocials, PhoneUnlinked}:     to(AuthAwaitingPhone),
		{AuthOnboardingCompleted, PhoneUnlinked}: to(AuthAwaitingPhone),
		{AuthOnboardingCompleted, XUnlinked}:     to(AuthAwaitingSocials),
	}
}

func to(next AuthState) func(AuthEvent) AuthState {
	return func(AuthEvent) AuthState { return next }
}

func afterPhone(ev AuthEvent) AuthState {
	if ev.HasX {
		return AuthOnboardingCompleted
	}
	return AuthAwaitingSocials
}

func xBeforePhone(ev AuthEvent) AuthState {
	if ev.HasPhone {
		return AuthCreated
	}
	return AuthAwaitingPhone
}

func NextAuthState(from AuthState, ev AuthEvent) (AuthState, error) {
	if !from.known() || !ev.Kind.known() {
		return from, errs.New(errs.CodeInternal, "identity.NextAuthState",
			slog.String("from", string(from)), slog.String("event", string(ev.Kind)))
	}
	step, ok := authSteps()[authStep{from, ev.Kind}]
	if !ok {
		return from, nil
	}
	return step(ev), nil
}
