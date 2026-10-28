package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type PreviewFundPot interface {
	TotalShares(ctx context.Context, cabalID ids.CabalID) (money.SharesUnits, error)
	PotValue(ctx context.Context, cabalID ids.CabalID) (money.Micros, error)
	MemberUnits(ctx context.Context, cabalID ids.CabalID, user ids.UserID) ([]money.SharesUnits, int, error)
}

type FundHistory interface {
	FirstFundedAt(ctx context.Context, cabalID ids.CabalID, user ids.UserID) (time.Time, bool, error)
}

type PreviewFundDeps struct {
	Cabals   FundCabals
	Balances fundingport.Balances
	Pauses   fundingport.Pauses
	Pot      PreviewFundPot
	History  FundHistory
}

type PreviewFundHandler struct{ d PreviewFundDeps }

func NewPreviewFundHandler(d PreviewFundDeps) *PreviewFundHandler { return &PreviewFundHandler{d: d} }

type FundPreview struct {
	AvailableMicros money.Micros
	MinMicros       money.Micros
	SliceBpsNow     int32
	SliceBpsAfter   *int32
	OverMicros      money.Micros
	Paused          bool
	FirstFundedAt   *time.Time
}

func (h *PreviewFundHandler) Handle(
	ctx context.Context, caller ids.UserID, cabal ids.CabalID, amount *money.Micros,
) (FundPreview, error) {
	if err := h.admit(ctx, caller, cabal); err != nil {
		return FundPreview{}, err
	}
	out, units, me, err := h.read(ctx, caller, cabal)
	if err != nil || amount == nil {
		return out, err
	}
	spend, over := capToCash(*amount, out.AvailableMicros)
	out.OverMicros = over
	if spend.Cmp(out.MinMicros) < 0 {
		return out, nil
	}
	after, priced, err := h.sliceAfter(ctx, cabal, units, me, spend)
	if err == nil && priced {
		out.SliceBpsAfter = &after
	}
	return out, err
}

func capToCash(amount, available money.Micros) (money.Micros, money.Micros) {
	over, err := amount.Sub(available)
	if err != nil || over.IsZero() {
		return amount, money.Micros{}
	}
	return available, over
}

func (h *PreviewFundHandler) read(
	ctx context.Context, caller ids.UserID, cabal ids.CabalID,
) (FundPreview, []money.SharesUnits, int, error) {
	pause, err := h.d.Pauses.IsPaused(ctx, cabal)
	if err != nil {
		return FundPreview{}, nil, 0, err
	}
	balance, err := h.d.Balances.Available(ctx, caller)
	if err != nil {
		return FundPreview{}, nil, 0, err
	}
	first, found, err := h.d.History.FirstFundedAt(ctx, cabal, caller)
	if err != nil {
		return FundPreview{}, nil, 0, err
	}
	units, me, err := h.d.Pot.MemberUnits(ctx, cabal, caller)
	if err != nil {
		return FundPreview{}, nil, 0, err
	}
	now, err := domain.SlicesBps(units)
	if err != nil {
		return FundPreview{}, nil, 0, err
	}
	out := FundPreview{
		AvailableMicros: balance.AvailableMicros, MinMicros: money.MicrosFromUint64(domain.MinFundMicros),
		SliceBpsNow: now[me], Paused: pause.Paused,
	}
	if found {
		out.FirstFundedAt = &first
	}
	return out, units, me, nil
}

func (h *PreviewFundHandler) admit(ctx context.Context, caller ids.UserID, cabal ids.CabalID) error {
	const op = "treasury.PreviewFund"
	member, err := h.d.Cabals.IsMember(ctx, cabal, caller)
	if err != nil {
		return err
	}
	if !member {
		return errs.New(errs.CodeNotCabalMember, op, slog.String("cabal_id", cabal.String()))
	}
	status, err := h.d.Cabals.Status(ctx, cabal)
	if err != nil {
		return err
	}
	if status == cabalport.StatusBanned {
		return errs.New(errs.CodeCabalBanned, op, slog.String("cabal_id", cabal.String()))
	}
	return nil
}

func (h *PreviewFundHandler) sliceAfter(
	ctx context.Context, cabal ids.CabalID, units []money.SharesUnits, me int, spend money.Micros,
) (int32, bool, error) {
	total, err := h.d.Pot.TotalShares(ctx, cabal)
	if err != nil {
		return 0, false, err
	}
	pot, err := h.d.Pot.PotValue(ctx, cabal)
	if errs.CodeOf(err) == errs.CodePriceUnavailable {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	bps, err := domain.SliceAfterFund(units, me, total, pot, spend)
	return bps, err == nil, err
}
