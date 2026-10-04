package adapters_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func TestUnwiredSignatureOwner_DeniesEverySignature(t *testing.T) {
	t.Parallel()
	owned, err := (adapters.UnwiredSignatureOwner{}).OwnsSignature(t.Context(), chain.Signature("signature"))
	if err != nil || owned {
		t.Fatalf("OwnsSignature() = %t, %v, want false, nil", owned, err)
	}
}
