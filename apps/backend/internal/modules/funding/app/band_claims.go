package app

import (
	"context"

	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func claimBand(
	ctx context.Context, from fundingport.ArrivalExpectation, bands []fundingport.ArrivalBand,
	sig chain.Signature, got money.Micros,
) (bool, error) {
	for _, b := range bands {
		if b.Claimed == sig && b.Contains(got) {
			return true, nil
		}
	}
	for _, b := range bands {
		if b.Claimed != "" || !b.Contains(got) {
			continue
		}
		claimed, err := from.ClaimArrival(ctx, b.ID, sig)
		if err != nil || claimed {
			return claimed, err
		}
	}
	return false, nil
}
