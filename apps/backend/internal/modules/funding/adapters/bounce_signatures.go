package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type BounceSignatures struct{ q *sqlc.Queries }

var _ port.SignatureOwner = BounceSignatures{}

func NewBounceSignatures(db sqlc.DBTX) BounceSignatures { return BounceSignatures{q: sqlc.New(db)} }

func (b BounceSignatures) OwnsSignature(ctx context.Context, sig chain.Signature) (bool, error) {
	owned, err := b.q.OwnsBounceSignature(ctx, string(sig))
	if err != nil {
		return false, errs.Wrap(err, errs.CodeInternal, "funding.BounceSignatures.OwnsSignature")
	}
	return owned, nil
}
