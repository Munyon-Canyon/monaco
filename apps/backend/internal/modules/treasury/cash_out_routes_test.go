package treasury_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	treasuryapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/treasuryapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func TestCashOutHTTP_startsPreviewsAndReads(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	u, c, err := f.fund(user, cabal, 1_000_000, 100, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	h := adapters.HTTP{CashOut: cashOutHandler(f, false)}
	ctx := auth.WithActor(f.ctx(), auth.Actor{Kind: auth.ActorUser, ID: user.String()})
	preview, err := h.GetCashOutPreview(ctx, treasuryapi.GetCashOutPreviewRequestObject{Id: cabal.UUID()})
	if err != nil {
		t.Fatal(err)
	}
	if got := preview.(treasuryapi.GetCashOutPreview200JSONResponse); got.SliceMicros != "1000000" {
		t.Fatalf("preview = %#v", got)
	}
	paused, err := adapters.HTTP{CashOut: cashOutHandler(f, true)}.GetCashOutPreview(
		ctx, treasuryapi.GetCashOutPreviewRequestObject{Id: cabal.UUID()},
	)
	if err != nil || paused.(treasuryapi.GetCashOutPreview200JSONResponse).Pause == nil {
		t.Fatalf("paused preview = %#v, %v", paused, err)
	}
	all := true
	started, err := h.PostCashOut(ctx, treasuryapi.PostCashOutRequestObject{
		Id: cabal.UUID(), Params: treasuryapi.PostCashOutParams{IdempotencyKey: "cash-out-http"},
		Body: &treasuryapi.CashOutRequest{All: &all},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := started.(treasuryapi.PostCashOut202JSONResponse)
	read, err := h.GetCashOutJob(ctx, treasuryapi.GetCashOutJobRequestObject{Id: cabal.UUID(), JobId: stored.Id})
	if err != nil {
		t.Fatal(err)
	}
	if got := read.(treasuryapi.GetCashOutJob200JSONResponse); got.Status !=
		treasuryapi.CashOutJobStatus(domain.CashOutStarted) {
		t.Fatalf("job = %#v", got)
	}
}

func TestCashOutHTTP_handlesErrors(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	h := adapters.HTTP{}
	if _, err := h.GetCashOutPreview(t.Context(), treasuryapi.GetCashOutPreviewRequestObject{}); err == nil {
		t.Fatal("preview caller error = nil")
	}
	if _, err := h.GetCashOutJob(t.Context(), treasuryapi.GetCashOutJobRequestObject{}); err == nil {
		t.Fatal("job caller error = nil")
	}
	if _, err := h.PostCashOut(t.Context(), treasuryapi.PostCashOutRequestObject{}); err == nil {
		t.Fatal("post caller error = nil")
	}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: f.user(t).String()})
	all, no := true, false
	bad, amount := "bad", "100000"
	for _, body := range []*treasuryapi.CashOutRequest{
		nil, {All: &no}, {All: &all, UsdcMicros: &amount}, {UsdcMicros: &bad},
	} {
		_, err := h.PostCashOut(ctx, treasuryapi.PostCashOutRequestObject{Body: body})
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("PostCashOut error = %v", err)
		}
	}
	h.CashOut = cashOutHandlerWith(f, cashOutPauses{err: errs.New(errs.CodeInternal, "test")}, newQueries(f))
	_, err := h.GetCashOutPreview(ctx, treasuryapi.GetCashOutPreviewRequestObject{})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("preview error = %v", err)
	}
	h.CashOut = cashOutHandler(f, true)
	if _, err := h.GetCashOutJob(ctx, treasuryapi.GetCashOutJobRequestObject{}); errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("job error = %v", err)
	}
	_, err = h.PostCashOut(ctx, treasuryapi.PostCashOutRequestObject{
		Body: &treasuryapi.CashOutRequest{UsdcMicros: &amount},
	})
	if errs.CodeOf(err) != errs.CodeCabalPaused {
		t.Fatalf("post error = %v", err)
	}
}

func TestCashOutHTTP_hidesAnotherUsersJob(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	owner, other, cabal := f.user(t), f.user(t), f.cabal(t)
	cashOutFund(t, f, owner, cabal, 1_000_000)
	h := adapters.HTTP{CashOut: cashOutHandler(f, false)}
	started, err := h.CashOut.Handle(
		observability.WithActor(f.ctx(), "user:"+owner.String()),
		app.CashOut{CabalID: cabal, UserID: owner, All: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	req := treasuryapi.GetCashOutJobRequestObject{Id: cabal.UUID(), JobId: started.ID}
	asOther := auth.WithActor(f.ctx(), auth.Actor{Kind: auth.ActorUser, ID: other.String()})
	if _, err := h.GetCashOutJob(asOther, req); errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("other user's job error = %v", err)
	}
	asOwner := auth.WithActor(f.ctx(), auth.Actor{Kind: auth.ActorUser, ID: owner.String()})
	if _, err := h.GetCashOutJob(asOwner, req); err != nil {
		t.Fatalf("owner's job error = %v", err)
	}
}

func TestCashOutHTTP_rejectsInvalidRequest(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	h := adapters.HTTP{}
	ctx := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorUser, ID: f.user(t).String()})
	if _, err := h.PostCashOut(ctx, treasuryapi.PostCashOutRequestObject{}); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("PostCashOut error = %v", err)
	}
}
