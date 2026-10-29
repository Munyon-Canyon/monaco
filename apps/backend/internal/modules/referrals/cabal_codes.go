package referrals

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type cabalCodes struct{ m *Module }

func (c cabalCodes) Resolve(ctx context.Context, code string) (ids.UserID, bool, error) {
	resolved, err := c.m.Resolver().Resolve(ctx, code)
	switch {
	case errs.CodeOf(err) == errs.CodeReferralCodeUnknown:
		return ids.UserID{}, false, nil
	case err != nil:
		return ids.UserID{}, false, err
	}
	return resolved.UserID, true, nil
}

func (m *Module) Codes() interface {
	Resolve(ctx context.Context, code string) (ids.UserID, bool, error)
} {
	return cabalCodes{m: m}
}
