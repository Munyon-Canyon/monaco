package chain

import "context"

type SignatureOwnerFunc func(context.Context, Signature) (bool, error)

func (f SignatureOwnerFunc) OwnsSignature(ctx context.Context, sig Signature) (bool, error) {
	return f(ctx, sig)
}
