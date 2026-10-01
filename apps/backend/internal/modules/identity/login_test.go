package identity_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
)

func TestPickLoginProvider_prefersPhoneThenEmailThenAppleThenGoogle(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		methods domain.LoginMethods
		want    domain.LoginProvider
	}{
		"phone only":         {domain.LoginMethods{Phone: true}, domain.LoginSMS},
		"email only":         {domain.LoginMethods{Email: true}, domain.LoginEmail},
		"apple only":         {domain.LoginMethods{Apple: true}, domain.LoginApple},
		"google only":        {domain.LoginMethods{Google: true}, domain.LoginGoogle},
		"phone and email":    {domain.LoginMethods{Phone: true, Email: true}, domain.LoginSMS},
		"email and apple":    {domain.LoginMethods{Email: true, Apple: true}, domain.LoginEmail},
		"apple and google":   {domain.LoginMethods{Apple: true, Google: true}, domain.LoginApple},
		"every method":       {domain.LoginMethods{Phone: true, Email: true, Apple: true, Google: true}, domain.LoginSMS},
		"phone after google": {domain.LoginMethods{Phone: true, Google: true}, domain.LoginSMS},
	} {
		got, err := domain.PickLoginProvider(tc.methods)
		if err != nil || got != tc.want {
			t.Errorf("%s: PickLoginProvider = %q, %v, want %q", name, got, err, tc.want)
		}
	}
}

func TestPickLoginProvider_refusesAUserWithNoLoginMethod(t *testing.T) {
	t.Parallel()
	got, err := domain.PickLoginProvider(domain.LoginMethods{})
	if got != "" || errs.CodeOf(err) != errs.CodeLoginMethodNotAllowed {
		t.Fatalf("PickLoginProvider = %q, %v, want login_method_not_allowed", got, err)
	}
}
