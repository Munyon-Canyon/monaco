package cabal_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func pot(units uint64) money.Micros { return money.MicrosFromUint64(units) }

func shares(units uint64) money.SharesUnits { return money.SharesUnitsFromUint64(units) }

func TestCheckLeave_refusesInTheOrderSharesThenLastMemberThenCreator(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		in   domain.LeaveInput
		want errs.Code
	}{
		{"a member with no shares and others left", domain.LeaveInput{MemberCount: 3, PotValue: pot(9)}, ""},
		{"the last member of an empty pot", domain.LeaveInput{MemberCount: 1}, ""},
		{"the creator as the last member of an empty pot", domain.LeaveInput{IsCreator: true, MemberCount: 1}, ""},
		{"a member holding shares", domain.LeaveInput{MemberCount: 3, ShareUnits: shares(1)}, errs.CodeLeaveHoldsShares},
		{"a member holding a single share of an empty pot", domain.LeaveInput{MemberCount: 2, ShareUnits: shares(1)}, errs.CodeLeaveHoldsShares},
		{"the last member with shares and a full pot", domain.LeaveInput{MemberCount: 1, ShareUnits: shares(5), PotValue: pot(1)}, errs.CodeLeaveHoldsShares},
		{"the creator with shares and members left", domain.LeaveInput{IsCreator: true, MemberCount: 2, ShareUnits: shares(5)}, errs.CodeLeaveHoldsShares},
		{"the last member of a pot worth one micro", domain.LeaveInput{MemberCount: 1, PotValue: pot(1)}, errs.CodeLeaveLastMemberPotNotEmpty},
		{"the last creator of a full pot", domain.LeaveInput{IsCreator: true, MemberCount: 1, PotValue: pot(1)}, errs.CodeLeaveLastMemberPotNotEmpty},
		{"the creator with one member left", domain.LeaveInput{IsCreator: true, MemberCount: 2}, errs.CodeLeaveCreatorWithMembers},
		{"the creator with a full pot and members left", domain.LeaveInput{IsCreator: true, MemberCount: 3, PotValue: pot(9)}, errs.CodeLeaveCreatorWithMembers},
		{"no members at all", domain.LeaveInput{}, errs.CodeInternal},
		{"a negative member count", domain.LeaveInput{MemberCount: -1, ShareUnits: shares(1)}, errs.CodeInternal},
	} {
		err := domain.CheckLeave(tt.in)
		if tt.want == "" {
			if err != nil {
				t.Errorf("%s: %v, want the leave allowed", tt.name, err)
			}
			continue
		}
		wantCode(t, tt.name, err, tt.want)
	}
}

func wantLeaveCode(in domain.LeaveInput) errs.Code {
	switch {
	case in.MemberCount < 1:
		return errs.CodeInternal
	case !in.ShareUnits.IsZero():
		return errs.CodeLeaveHoldsShares
	case in.MemberCount == 1 && in.PotValue.Uint64() > 0:
		return errs.CodeLeaveLastMemberPotNotEmpty
	case in.IsCreator && in.MemberCount > 1:
		return errs.CodeLeaveCreatorWithMembers
	}
	return ""
}

func TestCheckLeave_returnsTheFirstFailingGuardForAnyInput(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		in := domain.LeaveInput{
			IsCreator:   rapid.Bool().Draw(t, "creator"),
			MemberCount: rapid.IntRange(-1, 4).Draw(t, "members"),
			ShareUnits:  shares(rapid.OneOf(rapid.Just(uint64(0)), rapid.Uint64()).Draw(t, "shares")),
			PotValue:    pot(rapid.OneOf(rapid.Just(uint64(0)), rapid.Uint64()).Draw(t, "pot")),
		}
		err := domain.CheckLeave(in)
		if want := wantLeaveCode(in); (want == "") != (err == nil) || (err != nil && errs.CodeOf(err) != want) {
			t.Fatalf("CheckLeave(%+v) = %v, want code %q", in, err, want)
		}
	})
}
