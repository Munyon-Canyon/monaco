package domain_test

import (
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func usd(v uint64) money.Micros { return money.MicrosFromUint64(v) }

func shares(v uint64) money.SharesUnits { return money.SharesUnitsFromUint64(v) }

func TestCabalNAV(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		in   domain.NAVInput
		want domain.NAV
	}{
		"cash only": {
			in:   domain.NAVInput{USDC: usd(5_000_000), TotalShares: shares(2_000_000)},
			want: domain.NAV{Value: usd(5_000_000), PerShare: usd(2_500_000)},
		},
		"cash reserved for cash-outs is not the pot's": {
			in:   domain.NAVInput{USDC: usd(5_000_000), Reserved: usd(1_000_000), TotalShares: shares(2_000_000)},
			want: domain.NAV{Value: usd(4_000_000), PerShare: usd(2_000_000)},
		},
		"no shares prices nothing per share": {
			in:   domain.NAVInput{USDC: usd(7)},
			want: domain.NAV{Value: usd(7)},
		},
		"holdings scale by their decimals": {
			in: domain.NAVInput{
				USDC: usd(1_000_000),
				Holdings: []domain.Holding{
					{AssetID: "AAPLx", Units: money.NewBaseUnits(1_500_000_000, 9), Price: usd(200_000_000)},
					{AssetID: "TSLAx", Units: money.NewBaseUnits(3, 0), Price: usd(10)},
				},
				TotalShares: shares(301_000_030),
			},
			want: domain.NAV{Value: usd(301_000_030), PerShare: usd(1_000_000)},
		},
		"each holding rounds down": {
			in: domain.NAVInput{Holdings: []domain.Holding{
				{Units: money.NewBaseUnits(1, 6), Price: usd(999_999)},
				{Units: money.NewBaseUnits(1, 6), Price: usd(1_999_999)},
			}},
			want: domain.NAV{Value: usd(1)},
		},
		"per share rounds down": {
			in:   domain.NAVInput{USDC: usd(10), TotalShares: shares(3_000_000)},
			want: domain.NAV{Value: usd(10), PerShare: usd(3)},
		},
		"nineteen decimals still fit": {
			in: domain.NAVInput{
				Holdings: []domain.Holding{{Units: money.NewBaseUnits(math.MaxUint64, 19), Price: usd(10)}},
			},
			want: domain.NAV{Value: usd(18)},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := domain.CabalNAV(tt.in)
			if err != nil || got != tt.want {
				t.Fatalf("CabalNAV = %+v, %v, want %+v", got, err, tt.want)
			}
		})
	}
}

func TestCabalNAV_Errors(t *testing.T) {
	t.Parallel()
	tests := map[string]domain.NAVInput{
		"decimals past uint64": {Holdings: []domain.Holding{{Units: money.NewBaseUnits(1, 20), Price: usd(1)}}},
		"holding overflows": {Holdings: []domain.Holding{
			{Units: money.NewBaseUnits(math.MaxUint64, 0), Price: usd(2)},
		}},
		"sum overflows": {USDC: usd(math.MaxUint64), Holdings: []domain.Holding{
			{Units: money.NewBaseUnits(1, 0), Price: usd(1)},
		}},
		"per share overflows": {USDC: usd(math.MaxUint64), TotalShares: shares(1)},
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got, err := domain.CabalNAV(in); errs.CodeOf(err) != errs.CodeInvalidInput || got != (domain.NAV{}) {
				t.Fatalf("CabalNAV = %+v, %v, want invalid_input and zero NAV", got, err)
			}
		})
	}
}

func TestCabalNAV_ReservationOverTheValueBreaksConservation(t *testing.T) {
	t.Parallel()
	got, err := domain.CabalNAV(domain.NAVInput{USDC: usd(1), Reserved: usd(2)})
	if errs.CodeOf(err) != errs.CodeConservationBroken || got != (domain.NAV{}) {
		t.Fatalf("CabalNAV = %+v, %v, want conservation_broken and zero NAV", got, err)
	}
}
