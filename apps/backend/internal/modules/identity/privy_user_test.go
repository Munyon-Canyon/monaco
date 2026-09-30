package identity_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
)

func TestPrivyUser_loginMethodsAreTheLinkedAccountsThatSignTheUserIn(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		user app.PrivyUser
		want domain.LoginMethods
	}{
		"nothing linked":   {app.PrivyUser{}, domain.LoginMethods{}},
		"phone":            {app.PrivyUser{PhoneE164: "+14155550100"}, domain.LoginMethods{Phone: true}},
		"email":            {app.PrivyUser{Email: "a@example.com"}, domain.LoginMethods{Email: true}},
		"apple":            {app.PrivyUser{AppleEmail: "a@privaterelay.appleid.com"}, domain.LoginMethods{Apple: true}},
		"google":           {app.PrivyUser{GoogleEmail: "a@gmail.com"}, domain.LoginMethods{Google: true}},
		"x is not a login": {app.PrivyUser{X: &domain.XAccount{UserID: "1", Username: "a"}}, domain.LoginMethods{}},
	} {
		if got := tc.user.LoginMethods(); got != tc.want {
			t.Errorf("%s: LoginMethods = %+v, want %+v", name, got, tc.want)
		}
	}
}

func TestPrivyUser_contactEmailIsTheFirstLinkedEmail(t *testing.T) {
	t.Parallel()
	all := app.PrivyUser{Email: "otp@example.com", AppleEmail: "apple@example.com", GoogleEmail: "google@example.com"}
	for name, tc := range map[string]struct {
		user app.PrivyUser
		want string
	}{
		"none":             {app.PrivyUser{PhoneE164: "+14155550100"}, ""},
		"every email":      {all, "otp@example.com"},
		"apple and google": {app.PrivyUser{AppleEmail: "apple@example.com", GoogleEmail: "google@example.com"}, "apple@example.com"},
		"google only":      {app.PrivyUser{GoogleEmail: "google@example.com"}, "google@example.com"},
	} {
		if got := tc.user.ContactEmail(); got != tc.want {
			t.Errorf("%s: ContactEmail = %q, want %q", name, got, tc.want)
		}
	}
}
