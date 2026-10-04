package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type UnwiredSignatureOwner struct{}

var _ port.SignatureOwner = UnwiredSignatureOwner{}

func (UnwiredSignatureOwner) OwnsSignature(context.Context, chain.Signature) (bool, error) {
	return false, nil
}
