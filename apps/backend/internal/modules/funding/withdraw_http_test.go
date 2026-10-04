package funding_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/fundingapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestHTTPWithdraw(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	h := adapters.HTTP{Withdrawals: f.handler, IDs: testkit.NewIDs(11)}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: f.user.ID.String()})
	resp, err := h.Withdraw(ctx, api.WithdrawRequestObject{
		Body: &api.WithdrawJSONRequestBody{AmountMicros: "2000000", ToAddress: string(withdrawTo)},
	})
	accepted, ok := resp.(api.Withdraw202JSONResponse)
	if err != nil || !ok || accepted.Status != api.WithdrawalStatus("submitted") ||
		accepted.TxSignature != string(withdrawSig) {
		t.Fatalf("Withdraw = %#v, %v", resp, err)
	}
	assertSubmittedRow(t, f)
}

func TestHTTPWithdrawRefusals(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	h := adapters.HTTP{Withdrawals: f.handler, IDs: testkit.NewIDs(13)}
	user := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: f.user.ID.String()})
	for name, c := range map[string]struct {
		amount, to string
		admin      bool
		want       errs.Code
	}{
		"caller":     {amount: "2000000", to: string(withdrawTo), admin: true, want: errs.CodeForbidden},
		"amount":     {amount: "999999", to: string(withdrawTo), want: errs.CodeInvalidInput},
		"address":    {amount: "2000000", to: "not-an-address", want: errs.CodeInvalidAddress},
		"own wallet": {amount: "2000000", to: string(f.user.Address), want: errs.CodeWithdrawToOwnWallet},
	} {
		ctx := user
		if c.admin {
			ctx = auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorAdmin, ID: "ops"})
		}
		_, err := h.Withdraw(ctx, api.WithdrawRequestObject{
			Body: &api.WithdrawJSONRequestBody{AmountMicros: c.amount, ToAddress: c.to},
		})
		if errs.CodeOf(err) != c.want {
			t.Errorf("%s: err = %v, want %s", name, err, c.want)
		}
	}
}
