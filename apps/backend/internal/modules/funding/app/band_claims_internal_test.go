package app

import (
	"testing"

	"github.com/google/uuid"

	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func band(id byte, lo, hi uint64) fundingport.ArrivalBand {
	return fundingport.ArrivalBand{
		ID:  uuid.UUID{15: id},
		Min: money.MicrosFromUint64(lo),
		Max: money.MicrosFromUint64(hi),
	}
}

func TestBandClaims_aBridgeBackOwnsOneSignatureAndTheSameOneAgain(t *testing.T) {
	t.Parallel()
	c := &bandClaims{by: map[uuid.UUID]chain.Signature{}}
	bands := []fundingport.ArrivalBand{band(1, 90, 100)}
	got := money.MicrosFromUint64(95)

	if !c.claim(bands, "first", got) {
		t.Fatal("the first signature must take the bridge-back")
	}
	if !c.claim(bands, "first", got) {
		t.Fatal("the first signature must stay own when it is seen again")
	}
	if c.claim(bands, "second", got) {
		t.Fatal("a second signature must not claim a bridge-back already taken")
	}
}

func TestBandClaims_twoBridgeBacksClaimTwoTransfersAndAFinishedOneIsForgotten(t *testing.T) {
	t.Parallel()
	c := &bandClaims{by: map[uuid.UUID]chain.Signature{}}
	bands := []fundingport.ArrivalBand{band(1, 90, 100), band(2, 90, 100)}
	got := money.MicrosFromUint64(95)

	if !c.claim(bands, "a", got) || !c.claim(bands, "b", got) || c.claim(bands, "c", got) {
		t.Fatal("two bridge-backs must own exactly two transfers")
	}
	if c.claim(nil, "a", got) || len(c.by) != 0 {
		t.Fatalf("claims = %v, want them dropped once no bridge-back is in flight", c.by)
	}
}
