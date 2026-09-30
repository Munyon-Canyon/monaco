package identity_test

import (
	"fmt"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
)

type flag uint8

const (
	anyFlag flag = iota
	yes
	no
)

func (f flag) matches(v bool) bool { return f == anyFlag || (f == yes) == v }

type authMove struct {
	from     domain.AuthState
	kind     domain.AuthEventKind
	hasPhone flag
	hasX     flag
	to       domain.AuthState
}

func authMoves() []authMove {
	return []authMove{
		{domain.AuthCreated, domain.PhoneVerified, anyFlag, no, domain.AuthAwaitingSocials},
		{domain.AuthCreated, domain.PhoneVerified, anyFlag, yes, domain.AuthOnboardingCompleted},
		{domain.AuthCreated, domain.PhoneSkipped, anyFlag, anyFlag, domain.AuthAwaitingPhone},
		{domain.AuthCreated, domain.XLinked, no, anyFlag, domain.AuthAwaitingPhone},
		{domain.AuthAwaitingPhone, domain.XLinked, no, anyFlag, domain.AuthAwaitingPhone},
		{domain.AuthAwaitingPhone, domain.PhoneVerified, anyFlag, no, domain.AuthAwaitingSocials},
		{domain.AuthAwaitingPhone, domain.PhoneVerified, anyFlag, yes, domain.AuthOnboardingCompleted},
		{domain.AuthAwaitingSocials, domain.XLinked, anyFlag, anyFlag, domain.AuthOnboardingCompleted},
		{domain.AuthAwaitingSocials, domain.PhoneUnlinked, anyFlag, anyFlag, domain.AuthAwaitingPhone},
		{domain.AuthOnboardingCompleted, domain.PhoneUnlinked, anyFlag, anyFlag, domain.AuthAwaitingPhone},
		{domain.AuthOnboardingCompleted, domain.XUnlinked, anyFlag, anyFlag, domain.AuthAwaitingSocials},
	}
}

func wantAuthState(from domain.AuthState, ev domain.AuthEvent) domain.AuthState {
	for _, m := range authMoves() {
		if m.from == from && m.kind == ev.Kind && m.hasPhone.matches(ev.HasPhone) && m.hasX.matches(ev.HasX) {
			return m.to
		}
	}
	return from
}

type authCase struct {
	from domain.AuthState
	ev   domain.AuthEvent
}

func everyAuthEvent() []authCase {
	out := make([]authCase, 0, 80)
	for _, from := range []domain.AuthState{
		domain.AuthCreated, domain.AuthAwaitingPhone, domain.AuthAwaitingSocials, domain.AuthOnboardingCompleted,
	} {
		for _, kind := range []domain.AuthEventKind{
			domain.PhoneVerified, domain.PhoneSkipped, domain.XLinked, domain.PhoneUnlinked, domain.XUnlinked,
		} {
			for _, flags := range [][2]bool{{false, false}, {false, true}, {true, false}, {true, true}} {
				out = append(out, authCase{from, domain.AuthEvent{Kind: kind, HasPhone: flags[0], HasX: flags[1]}})
			}
		}
	}
	return out
}

func TestNextAuthState_everyStateEventAndLinkCombination(t *testing.T) {
	t.Parallel()
	all := everyAuthEvent()
	if len(all) != 80 {
		t.Fatalf("covered %d combinations, want 80", len(all))
	}
	for _, c := range all {
		want := wantAuthState(c.from, c.ev)
		t.Run(fmt.Sprintf("%s+%s/phone=%t/x=%t", c.from, c.ev.Kind, c.ev.HasPhone, c.ev.HasX), func(t *testing.T) {
			t.Parallel()
			got, err := domain.NextAuthState(c.from, c.ev)
			if err != nil {
				t.Fatalf("NextAuthState: %v", err)
			}
			if got != want {
				t.Errorf("NextAuthState = %s, want %s", got, want)
			}
		})
	}
}

func TestNextAuthState_unknownValuesFailInternal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		from domain.AuthState
		kind domain.AuthEventKind
	}{
		{"unknown state", domain.AuthState("LIMBO"), domain.PhoneVerified},
		{"unknown event", domain.AuthCreated, domain.AuthEventKind("email_linked")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := domain.NextAuthState(tt.from, domain.AuthEvent{Kind: tt.kind})
			if code := errs.CodeOf(err); err == nil || code != errs.CodeInternal {
				t.Fatalf("err = %v (code %s), want %s", err, code, errs.CodeInternal)
			}
			if got != tt.from {
				t.Errorf("state = %s, want it unchanged at %s", got, tt.from)
			}
		})
	}
}
