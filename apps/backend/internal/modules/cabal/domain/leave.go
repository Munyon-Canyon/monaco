package domain

import (
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type LeaveInput struct {
	IsCreator   bool
	MemberCount int
	ShareUnits  money.SharesUnits
	PotValue    money.Micros
}

func CheckLeave(in LeaveInput) error {
	const op = "cabal.CheckLeave"
	switch {
	case in.MemberCount < 1:
		return errs.New(errs.CodeInternal, op, slog.Int("members", in.MemberCount))
	case !in.ShareUnits.IsZero():
		return errs.New(errs.CodeLeaveHoldsShares, op, slog.Uint64("have", in.ShareUnits.Uint64()))
	case in.MemberCount == 1 && !in.PotValue.IsZero():
		return errs.New(errs.CodeLeaveLastMemberPotNotEmpty, op, slog.Uint64("have", in.PotValue.Uint64()))
	case in.IsCreator && in.MemberCount > 1:
		return errs.New(errs.CodeLeaveCreatorWithMembers, op, slog.Int("members", in.MemberCount))
	}
	return nil
}
