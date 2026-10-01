package identity_test

import (
	"reflect"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
)

const (
	phoneN1 = "+14155550100"
	phoneN2 = "+14155550101"
)

func xOne() *domain.XAccount { return &domain.XAccount{UserID: "x-1", Username: "one"} }

func xTwo() *domain.XAccount { return &domain.XAccount{UserID: "x-2", Username: "two"} }

func step(from, to domain.AuthState, cause domain.AuthCause) domain.AuthStep {
	return domain.AuthStep{From: from, To: to, Cause: cause}
}

func TestSyncLinks_followsWhatPrivyLinksAndUnlinks(t *testing.T) {
	t.Parallel()
	const (
		created = domain.AuthCreated
		phone   = domain.AuthAwaitingPhone
		socials = domain.AuthAwaitingSocials
		done    = domain.AuthOnboardingCompleted
	)
	for name, tc := range map[string]struct {
		from          domain.AuthState
		stored, privy domain.Links
		phone         domain.Write[string]
		x             domain.Write[*domain.XAccount]
		steps         []domain.AuthStep
	}{
		"nothing linked anywhere": {from: done},
		"everything unchanged": {
			from: done, stored: domain.Links{Phone: phoneN1, X: xOne()}, privy: domain.Links{Phone: phoneN1, X: xOne()},
		},
		"the phone is unlinked": {
			from: done, stored: domain.Links{Phone: phoneN1, X: xOne()}, privy: domain.Links{X: xOne()},
			phone: domain.Write[string]{Changed: true}, steps: []domain.AuthStep{step(done, phone, domain.CauseUnlink)},
		},
		"X is unlinked": {
			from: done, stored: domain.Links{Phone: phoneN1, X: xOne()}, privy: domain.Links{Phone: phoneN1},
			x: domain.Write[*domain.XAccount]{Changed: true}, steps: []domain.AuthStep{step(done, socials, domain.CauseUnlink)},
		},
		"both are unlinked, phone first": {
			from: done, stored: domain.Links{Phone: phoneN1, X: xOne()},
			phone: domain.Write[string]{Changed: true}, x: domain.Write[*domain.XAccount]{Changed: true},
			steps: []domain.AuthStep{step(done, phone, domain.CauseUnlink)},
		},
		"the phone is linked while X is stored": {
			from: phone, stored: domain.Links{X: xOne()}, privy: domain.Links{Phone: phoneN1, X: xOne()},
			phone: domain.Write[string]{Changed: true, Value: phoneN1},
			steps: []domain.AuthStep{step(phone, done, domain.CauseLink)},
		},
		"the phone is linked with no X": {
			from: phone, privy: domain.Links{Phone: phoneN1},
			phone: domain.Write[string]{Changed: true, Value: phoneN1},
			steps: []domain.AuthStep{step(phone, socials, domain.CauseLink)},
		},
		"X is linked after the phone": {
			from: socials, stored: domain.Links{Phone: phoneN1}, privy: domain.Links{Phone: phoneN1, X: xOne()},
			x:     domain.Write[*domain.XAccount]{Changed: true, Value: xOne()},
			steps: []domain.AuthStep{step(socials, done, domain.CauseLink)},
		},
		"X is linked before the phone": {
			from: phone, privy: domain.Links{X: xOne()}, x: domain.Write[*domain.XAccount]{Changed: true, Value: xOne()},
		},
		"both are linked in one go": {
			from: phone, privy: domain.Links{Phone: phoneN1, X: xOne()},
			phone: domain.Write[string]{Changed: true, Value: phoneN1},
			x:     domain.Write[*domain.XAccount]{Changed: true, Value: xOne()},
			steps: []domain.AuthStep{step(phone, socials, domain.CauseLink), step(socials, done, domain.CauseLink)},
		},
		"a replaced phone unlinks then links": {
			from: done, stored: domain.Links{Phone: phoneN1, X: xOne()}, privy: domain.Links{Phone: phoneN2, X: xOne()},
			phone: domain.Write[string]{Changed: true, Value: phoneN2},
			steps: []domain.AuthStep{step(done, phone, domain.CauseUnlink), step(phone, done, domain.CauseLink)},
		},
		"a replaced X unlinks then links": {
			from: done, stored: domain.Links{Phone: phoneN1, X: xOne()}, privy: domain.Links{Phone: phoneN1, X: xTwo()},
			x:     domain.Write[*domain.XAccount]{Changed: true, Value: xTwo()},
			steps: []domain.AuthStep{step(done, socials, domain.CauseUnlink), step(socials, done, domain.CauseLink)},
		},
		"a new username for the same X account changes nothing": {
			from: done, stored: domain.Links{Phone: phoneN1, X: xOne()},
			privy: domain.Links{Phone: phoneN1, X: &domain.XAccount{UserID: "x-1", Username: "renamed"}},
		},
		"during onboarding a new link is left to the onboarding commands": {
			from: created, privy: domain.Links{Phone: phoneN1, X: xOne()},
		},
		"during onboarding an unlink still clears the columns but moves no state": {
			from: created, stored: domain.Links{X: xOne()}, x: domain.Write[*domain.XAccount]{Changed: true},
		},
		"unlinking a phone that never moved the state": {
			from: phone, stored: domain.Links{Phone: phoneN1}, phone: domain.Write[string]{Changed: true},
		},
	} {
		got, err := domain.SyncLinks(tc.from, tc.stored, tc.privy)
		if err != nil || got.Phone != tc.phone || !reflect.DeepEqual(got.X, tc.x) ||
			!reflect.DeepEqual(got.Steps, tc.steps) {
			t.Errorf(
				"%s: SyncLinks = %+v, %v, want phone %+v, x %+v, steps %v",
				name,
				got,
				err,
				tc.phone,
				tc.x,
				tc.steps,
			)
		}
		if empty := tc.phone == (domain.Write[string]{}) && !tc.x.Changed &&
			len(tc.steps) == 0; got.Empty() != empty {
			t.Errorf("%s: Empty = %v, want %v", name, got.Empty(), empty)
		}
	}
}

func TestSyncLinks_anUnknownStateIsAnInternalError(t *testing.T) {
	t.Parallel()
	_, err := domain.SyncLinks(domain.AuthState("BOGUS"), domain.Links{Phone: phoneN1}, domain.Links{})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("SyncLinks from an unknown state = %v, want internal", err)
	}
	_, err = domain.SyncLinks(domain.AuthState("BOGUS"), domain.Links{Phone: phoneN1}, domain.Links{Phone: phoneN2})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("SyncLinks with two moves from an unknown state = %v, want the first error kept", err)
	}
}

func TestLinks_claimsAreTheAccountsNewToTheUserOutsideOnboarding(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		privy, stored domain.Links
		from          domain.AuthState
		want          domain.Claims
	}{
		"a new phone and X": {
			privy: domain.Links{Phone: phoneN1, X: xOne()}, from: domain.AuthAwaitingPhone,
			want: domain.Claims{Phone: true, X: true},
		},
		"already stored": {
			privy: domain.Links{Phone: phoneN1, X: xOne()}, stored: domain.Links{Phone: phoneN1, X: xOne()},
			from: domain.AuthOnboardingCompleted,
		},
		"a replaced phone and X": {
			privy: domain.Links{Phone: phoneN2, X: xTwo()}, stored: domain.Links{Phone: phoneN1, X: xOne()},
			from: domain.AuthOnboardingCompleted, want: domain.Claims{Phone: true, X: true},
		},
		"nothing linked":    {from: domain.AuthAwaitingPhone},
		"during onboarding": {privy: domain.Links{Phone: phoneN1, X: xOne()}, from: domain.AuthCreated},
	} {
		got := tc.privy.Claims(tc.stored, tc.from)
		if got != tc.want || got.Any() != (tc.want.Phone || tc.want.X) {
			t.Errorf("%s: Claims = %+v, want %+v", name, got, tc.want)
		}
	}
}

func TestLinks_withoutDropsTheHeldAccountsOnly(t *testing.T) {
	t.Parallel()
	links := domain.Links{Phone: phoneN1, X: xOne()}
	for name, tc := range map[string]struct {
		held domain.Claims
		want domain.Links
	}{
		"none":  {domain.Claims{}, links},
		"phone": {domain.Claims{Phone: true}, domain.Links{X: xOne()}},
		"x":     {domain.Claims{X: true}, domain.Links{Phone: phoneN1}},
		"both":  {domain.Claims{Phone: true, X: true}, domain.Links{}},
	} {
		if got := links.Without(tc.held); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: Without = %+v, want %+v", name, got, tc.want)
		}
	}
}

func TestPhoneHash_isTheSHA256OfTheE164Number(t *testing.T) {
	t.Parallel()
	got := domain.PhoneHash("+15550100")
	want := portHash("+15550100")
	if !reflect.DeepEqual(got, want) || len(got) != 32 {
		t.Fatalf("PhoneHash = %x, want %x", got, want)
	}
}
