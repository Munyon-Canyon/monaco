package app

import (
	"context"
	"testing"

	"github.com/google/uuid"

	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type neverClaims struct{ calls int }

func (*neverClaims) BridgeBacksInFlight(context.Context, ids.CabalID) ([]fundingport.ArrivalBand, error) {
	return nil, nil
}

func (n *neverClaims) ClaimArrival(context.Context, uuid.UUID, chain.Signature) (bool, error) {
	n.calls++
	return true, nil
}

func TestClaimBand_aSignatureAlreadyHoldingABandStaysOwnWithoutAnotherWrite(t *testing.T) {
	t.Parallel()
	from := &neverClaims{}
	bands := []fundingport.ArrivalBand{{
		ID: uuid.UUID{15: 1}, Min: money.MicrosFromUint64(90), Max: money.MicrosFromUint64(100), Claimed: "first",
	}}

	own, err := claimBand(t.Context(), from, bands, "first", money.MicrosFromUint64(95))
	other, otherErr := claimBand(t.Context(), from, bands, "second", money.MicrosFromUint64(95))

	if err != nil || otherErr != nil || !own || other || from.calls != 0 {
		t.Fatalf("own = %t, other = %t, writes = %d, want first own, second refused, no write", own, other, from.calls)
	}
}
