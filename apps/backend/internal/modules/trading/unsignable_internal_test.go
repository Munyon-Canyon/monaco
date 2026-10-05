package trading

import (
	"errors"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func TestUnsignable_failsBothStepsWithItsConfigError(t *testing.T) {
	t.Parallel()
	cause := errs.New(errs.CodeInvalidInput, "test")
	u := unsignable{err: cause}
	payer, payErr := u.FeePayer()
	_, sig, signErr := u.Sign(t.Context(), "wallet", nil)
	if payer != "" || sig != "" || !errors.Is(payErr, cause) || !errors.Is(signErr, cause) {
		t.Fatalf("FeePayer = %q, %v; Sign = %q, %v; want both to fail with %v", payer, payErr, sig, signErr, cause)
	}
}
