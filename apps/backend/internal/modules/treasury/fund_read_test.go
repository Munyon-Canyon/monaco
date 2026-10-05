package treasury_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/treasuryapi"
)

func TestFundHTTP_readsATransferToItsOwnerOnly(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	srv := adapters.HTTP{FundReads: h.pool}
	read := func(ctx context.Context, id uuid.UUID) (api.GetFundTransfer200JSONResponse, error) {
		res, err := srv.GetFundTransfer(ctx, api.GetFundTransferRequestObject{Id: id})
		if err != nil {
			return api.GetFundTransfer200JSONResponse{}, err
		}
		return res.(api.GetFundTransfer200JSONResponse), nil
	}
	submitted, err := h.fund(5_000_000)
	if err != nil {
		t.Fatal(err)
	}
	got, err := read(h.actorCtx(), submitted)
	if err != nil || got.Status != "submitted" || got.AmountMicros != "5000000" || got.ShareUnits != nil ||
		got.FailCode != nil {
		t.Fatalf("submitted transfer = %#v, %v", got, err)
	}
	settled := h.fundTransferIn(t, h.user, domain.FundSettled, 5_000_000)
	if got, err := read(h.actorCtx(), settled); err != nil || *got.ShareUnits != "7" {
		t.Fatalf("settled transfer = %#v, %v, want 7 share units", got, err)
	}
	failed := h.fundTransferIn(t, h.user, domain.FundFailed, 5_000_000)
	if got, err := read(h.actorCtx(), failed); err != nil || *got.FailCode != "fund_not_sent" {
		t.Fatalf("failed transfer = %#v, %v, want fund_not_sent", got, err)
	}
	_, err = read(h.ctx(), submitted)
	wantCode(t, err, errs.CodeUnauthorized)
	_, err = read(h.actorCtx(), h.ids.NewV7())
	wantCode(t, err, errs.CodeNotFound)
	_, err = read(h.actorCtx(), h.fundTransferIn(t, h.fixture.user(t), domain.FundSubmitted, 5_000_000))
	wantCode(t, err, errs.CodeNotFound)
	_, err = read(canceled(h.actorCtx()), submitted)
	wantCode(t, err, errs.CodeInternal)
}
