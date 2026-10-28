package app

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	dbsqlc "github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type previewFake struct {
	member    bool
	status    cabalport.Status
	paused    bool
	available uint64
	units     []money.SharesUnits
	me        int
	total     uint64
	pot       uint64
	first     *time.Time
	failOn    string
	potErr    error
}

func (f *previewFake) fail(op string) error {
	if f.failOn == op {
		return errs.New(errs.CodeInternal, "fake."+op)
	}
	return nil
}

func (f *previewFake) IsMember(context.Context, ids.CabalID, ids.UserID) (bool, error) {
	return f.member, f.fail("IsMember")
}

func (f *previewFake) LockMember(context.Context, dbsqlc.DBTX, ids.CabalID, ids.UserID) (bool, error) {
	return f.member, nil
}

func (f *previewFake) Status(context.Context, ids.CabalID) (cabalport.Status, error) {
	return f.status, f.fail("Status")
}

func (f *previewFake) TreasuryWallet(context.Context, ids.CabalID) (cabalport.TreasuryWallet, error) {
	return cabalport.TreasuryWallet{}, nil
}

func (f *previewFake) Available(context.Context, ids.UserID) (fundingport.Balance, error) {
	return fundingport.Balance{AvailableMicros: money.MicrosFromUint64(f.available)}, f.fail("Available")
}

func (f *previewFake) Observe(context.Context, ids.UserID) (fundingport.Observation, error) {
	return fundingport.Observation{}, nil
}

func (f *previewFake) AvailableIn(context.Context, dbsqlc.DBTX, fundingport.Observation) (fundingport.Balance, error) {
	return fundingport.Balance{}, nil
}

func (f *previewFake) IsPaused(context.Context, ids.CabalID) (fundingport.Pause, error) {
	return fundingport.Pause{Paused: f.paused}, f.fail("IsPaused")
}

func (f *previewFake) PausedCabals(context.Context) (fundingport.PausedSet, error) {
	return fundingport.PausedSet{}, nil
}

func (f *previewFake) MemberUnits(context.Context, ids.CabalID, ids.UserID) ([]money.SharesUnits, int, error) {
	return f.units, f.me, f.fail("MemberUnits")
}

func (f *previewFake) TotalShares(context.Context, ids.CabalID) (money.SharesUnits, error) {
	return money.SharesUnitsFromUint64(f.total), f.fail("TotalShares")
}

func (f *previewFake) PotValue(context.Context, ids.CabalID) (money.Micros, error) {
	if f.potErr != nil {
		return money.Micros{}, f.potErr
	}
	return money.MicrosFromUint64(f.pot), f.fail("PotValue")
}

func (f *previewFake) FirstFundedAt(context.Context, ids.CabalID, ids.UserID) (time.Time, bool, error) {
	if f.first == nil {
		return time.Time{}, false, f.fail("FirstFundedAt")
	}
	return *f.first, true, f.fail("FirstFundedAt")
}

func newPreviewFake() *previewFake {
	return &previewFake{
		member: true, status: cabalport.StatusActive, available: 500_000_000,
		units: []money.SharesUnits{money.SharesUnitsFromUint64(35_000_000), money.SharesUnitsFromUint64(65_000_000)},
		total: 100_000_000, pot: 100_000_000,
	}
}

func (f *previewFake) handler() *PreviewFundHandler {
	return NewPreviewFundHandler(PreviewFundDeps{Cabals: f, Balances: f, Pauses: f, Pot: f, History: f})
}

func previewAmount(v uint64) *money.Micros {
	m := money.MicrosFromUint64(v)
	return &m
}

func TestPreviewFund(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 3, 15, 0, 0, 0, time.UTC)
	tests := map[string]struct {
		arrange   func(*previewFake)
		amount    *money.Micros
		wantAfter *int32
		wantOver  uint64
		wantNow   int32
		paused    bool
		first     bool
	}{
		"no amount":         {amount: nil, wantNow: 3500},
		"zero amount":       {amount: previewAmount(0), wantNow: 3500},
		"below the minimum": {amount: previewAmount(999_999), wantNow: 3500},
		"within the cash":   {amount: previewAmount(1_562_500), wantNow: 3500, wantAfter: bps(3600)},
		"over the cash": {
			arrange:   func(f *previewFake) { f.available = 1_562_500 },
			amount:    previewAmount(650_000_000),
			wantNow:   3500,
			wantAfter: bps(3600),
			wantOver:  648_437_500,
		},
		"over with no cash": {
			arrange: func(f *previewFake) { f.available = 0 }, amount: previewAmount(5_000_000), wantNow: 3500,
			wantOver: 5_000_000,
		},
		"exactly the cash": {
			arrange: func(f *previewFake) { f.available = 1_562_500 }, amount: previewAmount(1_562_500), wantNow: 3500,
			wantAfter: bps(3600),
		},
		"paused":        {arrange: func(f *previewFake) { f.paused = true }, wantNow: 3500, paused: true},
		"funded before": {arrange: func(f *previewFake) { f.first = &at }, wantNow: 3500, first: true},
		"first funder": {
			arrange: func(f *previewFake) {
				f.units, f.me, f.total, f.pot = []money.SharesUnits{{}}, 0, 0, 0
			},
			amount: previewAmount(5_000_000), wantNow: 0, wantAfter: bps(10000),
		},
		"no price": {
			arrange: func(f *previewFake) { f.potErr = errs.New(errs.CodePriceUnavailable, "fake") },
			amount:  previewAmount(5_000_000), wantNow: 3500,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newPreviewFake()
			if tt.arrange != nil {
				tt.arrange(f)
			}
			got, err := f.handler().Handle(t.Context(), ids.UserID{}, ids.CabalID{}, tt.amount)
			if err != nil {
				t.Fatal(err)
			}
			if !sameAfter(got.SliceBpsAfter, tt.wantAfter) {
				t.Fatalf("after = %v, want %v", got.SliceBpsAfter, tt.wantAfter)
			}
			if got.SliceBpsNow != tt.wantNow || got.OverMicros.Uint64() != tt.wantOver || got.Paused != tt.paused ||
				(got.FirstFundedAt != nil) != tt.first || got.MinMicros.Uint64() != 1_000_000 {
				t.Fatalf("preview = %+v, want now %d over %d paused %t", got, tt.wantNow, tt.wantOver, tt.paused)
			}
		})
	}
}

func sameAfter(got, want *int32) bool {
	if got == nil || want == nil {
		return got == want
	}
	return *got == *want
}

func bps(v int32) *int32 { return &v }

func TestPreviewFund_refusals(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		arrange func(*previewFake)
		want    errs.Code
	}{
		"not a member": {arrange: func(f *previewFake) { f.member = false }, want: errs.CodeNotCabalMember},
		"banned": {
			arrange: func(f *previewFake) { f.status = cabalport.StatusBanned },
			want:    errs.CodeCabalBanned,
		},
		"member read":     {arrange: func(f *previewFake) { f.failOn = "IsMember" }, want: errs.CodeInternal},
		"status read":     {arrange: func(f *previewFake) { f.failOn = "Status" }, want: errs.CodeInternal},
		"pause read":      {arrange: func(f *previewFake) { f.failOn = "IsPaused" }, want: errs.CodeInternal},
		"balance read":    {arrange: func(f *previewFake) { f.failOn = "Available" }, want: errs.CodeInternal},
		"first fund read": {arrange: func(f *previewFake) { f.failOn = "FirstFundedAt" }, want: errs.CodeInternal},
		"units read":      {arrange: func(f *previewFake) { f.failOn = "MemberUnits" }, want: errs.CodeInternal},
		"total read":      {arrange: func(f *previewFake) { f.failOn = "TotalShares" }, want: errs.CodeInternal},
		"pot read":        {arrange: func(f *previewFake) { f.failOn = "PotValue" }, want: errs.CodeInternal},
		"pot worthless":   {arrange: func(f *previewFake) { f.pot = 0 }, want: errs.CodePotValueZero},
		"slices overflow": {
			arrange: func(f *previewFake) {
				f.units = []money.SharesUnits{
					money.SharesUnitsFromUint64(
						1 << 63,
					),
					money.SharesUnitsFromUint64(1 << 63),
					money.SharesUnitsFromUint64(1),
				}
			},
			want: errs.CodeInvalidInput,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newPreviewFake()
			tt.arrange(f)
			_, err := f.handler().Handle(t.Context(), ids.UserID{}, ids.CabalID{}, previewAmount(5_000_000))
			if errs.CodeOf(err) != tt.want {
				t.Fatalf("code = %q (%v), want %q", errs.CodeOf(err), err, tt.want)
			}
		})
	}
}
