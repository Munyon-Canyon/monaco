package domain

import "github.com/monaco/monaco/apps/backend/internal/errs"

type LoginMethods struct {
	Phone  bool
	Email  bool
	Apple  bool
	Google bool
}

func PickLoginProvider(m LoginMethods) (LoginProvider, error) {
	for _, c := range [...]struct {
		linked   bool
		provider LoginProvider
	}{
		{m.Phone, LoginSMS},
		{m.Email, LoginEmail},
		{m.Apple, LoginApple},
		{m.Google, LoginGoogle},
	} {
		if c.linked {
			return c.provider, nil
		}
	}
	return "", errs.New(errs.CodeLoginMethodNotAllowed, "identity.PickLoginProvider")
}
