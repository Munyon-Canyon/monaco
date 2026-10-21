package funding_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/fundingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func authorized(
	ctx context.Context, t *testing.T, h adapters.HTTP, body api.WithdrawJSONRequestBody,
) api.WithdrawRequestObject {
	t.Helper()
	resp, err := h.AuthorizeWithdrawal(ctx, api.AuthorizeWithdrawalRequestObject{Body: &body})
	granted, ok := resp.(api.AuthorizeWithdrawal201JSONResponse)
	if err != nil || !ok {
		t.Fatalf("AuthorizeWithdrawal = %#v, %v", resp, err)
	}
	return api.WithdrawRequestObject{
		Params: api.WithdrawParams{WithdrawalAuthorization: &granted.AuthorizationId}, Body: &body,
	}
}

func TestHTTPWithdrawThenRead(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	h := adapters.HTTP{
		Withdrawals: f.handler, Authorize: app.NewAuthorizeWithdrawalHandler(f.uow, f.clock), Reads: f.pool,
		IDs: testkit.NewIDs(11),
	}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: f.user.ID.String()})
	resp, err := h.Withdraw(ctx, authorized(ctx, t, h,
		api.WithdrawJSONRequestBody{AmountMicros: "2000000", ToAddress: string(withdrawTo)}))
	accepted, ok := resp.(api.Withdraw202JSONResponse)
	if err != nil || !ok || accepted.Status != api.WithdrawalStatus("submitted") ||
		accepted.TxSignature != string(withdrawSig) {
		t.Fatalf("Withdraw = %#v, %v", resp, err)
	}
	assertSubmittedRead(ctx, t, h, accepted.WithdrawalId, f.now)
	stranger := ids.UserIDFrom(ids.Real{}.NewV7()).String()
	other := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: stranger})
	_, err = h.GetMyWithdrawal(other, api.GetMyWithdrawalRequestObject{Id: accepted.WithdrawalId})
	if errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("another user's read err = %v, want not_found", err)
	}
}

func assertSubmittedRead(ctx context.Context, t *testing.T, h adapters.HTTP, id uuid.UUID, created time.Time) {
	t.Helper()
	read, err := h.GetMyWithdrawal(ctx, api.GetMyWithdrawalRequestObject{Id: id})
	got, ok := read.(api.GetMyWithdrawal200JSONResponse)
	if err != nil || !ok || got.Status != "submitted" || got.AmountMicros != "2000000" ||
		got.ToAddress != string(withdrawTo) || got.TxSignature == nil || *got.TxSignature != string(withdrawSig) ||
		got.FailCode != nil || got.CompletedAt != nil || !got.CreatedAt.Equal(created) {
		t.Fatalf("GetMyWithdrawal = %#v, %v", read, err)
	}
}

func TestHTTPReadFailedWithdrawal(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	f.transfers.buildErr = errs.New(errs.CodePrivyUnavailable, "test")
	h := adapters.HTTP{
		Withdrawals: f.handler, Authorize: app.NewAuthorizeWithdrawalHandler(f.uow, f.clock), Reads: f.pool,
		IDs: testkit.NewIDs(12),
	}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: f.user.ID.String()})
	_, err := h.Withdraw(ctx, authorized(ctx, t, h,
		api.WithdrawJSONRequestBody{AmountMicros: "2000000", ToAddress: string(withdrawTo)}))
	if errs.CodeOf(err) != errs.CodePrivyUnavailable {
		t.Fatalf("Withdraw err = %v", err)
	}
	var id uuid.UUID
	if err := f.pool.QueryRow(t.Context(), `SELECT id FROM withdrawals`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	read, err := h.GetMyWithdrawal(ctx, api.GetMyWithdrawalRequestObject{Id: id})
	got, ok := read.(api.GetMyWithdrawal200JSONResponse)
	if err != nil || !ok || got.Status != "failed" || got.FailCode == nil || *got.FailCode != "privy_unavailable" ||
		got.TxSignature != nil || got.CompletedAt == nil {
		t.Fatalf("GetMyWithdrawal = %#v, %v", read, err)
	}
}

func TestHTTPWithdrawRefusals(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	h := adapters.HTTP{Withdrawals: f.handler, Reads: f.pool, IDs: testkit.NewIDs(13)}
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
	admin := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorAdmin, ID: "ops"})
	if _, err := h.GetMyWithdrawal(admin, api.GetMyWithdrawalRequestObject{Id: ids.Real{}.NewV7()}); errs.CodeOf(err) !=
		errs.CodeForbidden {
		t.Fatalf("admin read err = %v", err)
	}
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE withdrawals RENAME TO withdrawals_gone`); err != nil {
		t.Fatal(err)
	}
	if _, err := h.GetMyWithdrawal(user, api.GetMyWithdrawalRequestObject{Id: ids.Real{}.NewV7()}); errs.CodeOf(err) !=
		errs.CodeInternal {
		t.Fatalf("broken read err = %v", err)
	}
}

func TestHTTPAuthorizeWithdrawalRefusals(t *testing.T) {
	t.Parallel()
	f := newWithdrawFixture(t, 5_000_000)
	h := adapters.HTTP{
		Authorize: app.NewAuthorizeWithdrawalHandler(f.uow, f.clock), Reads: f.pool, IDs: testkit.NewIDs(14),
	}
	body := func(amount, to string) api.AuthorizeWithdrawalRequestObject {
		return api.AuthorizeWithdrawalRequestObject{Body: &api.WithdrawRequest{AmountMicros: amount, ToAddress: to}}
	}
	if _, err := h.AuthorizeWithdrawal(t.Context(), body("2000000", string(withdrawTo))); errs.CodeOf(err) !=
		errs.CodeUnauthorized {
		t.Fatalf("without an actor err = %v, want unauthorized", err)
	}
	user := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: f.user.ID.String()})
	if _, err := h.AuthorizeWithdrawal(user, body("999999", string(withdrawTo))); errs.CodeOf(err) !=
		errs.CodeInvalidInput {
		t.Fatalf("below the minimum err = %v, want invalid_input", err)
	}
	stale := auth.WithActor(t.Context(), auth.Actor{
		Kind: auth.ActorUser, ID: f.user.ID.String(), SessionID: phoneSID,
		IssuedAt: f.clock.Now().Add(-time.Hour).Unix(),
	})
	if _, err := h.AuthorizeWithdrawal(stale, body("2000000", string(withdrawTo))); errs.CodeOf(err) !=
		errs.CodeReauthRequired {
		t.Fatalf("a copied token err = %v, want reauth_required", err)
	}
	if _, err := f.pool.Exec(
		t.Context(),
		`ALTER TABLE withdrawal_authorizations RENAME TO authorizations_gone`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := h.AuthorizeWithdrawal(user, body("2000000", string(withdrawTo))); err == nil {
		t.Fatal("a broken table = nil, want the database error")
	}
}
