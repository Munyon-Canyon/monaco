package port

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type SignatureOwner interface {
	OwnsSignature(ctx context.Context, sig chain.Signature) (bool, error)
}
