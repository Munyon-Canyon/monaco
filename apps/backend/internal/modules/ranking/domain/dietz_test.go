package domain_test

import (
	"math"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestModifiedDietz_MidRangeDeposit(t *testing.T) {
	t.Parallel()
	t1 := clock.Real{}.Now().UTC()
	t0 := t1.Add(-7 * 24 * time.Hour)
	gain, ret, err := domain.ModifiedDietz(domain.DietzInput{
		T0: t0, T1: t1, Start: usd(100_000_000), End: usd(200_000_000),
		Flows: []domain.Flow{{Amount: signed(100_000_000), At: t0.Add(3 * 24 * time.Hour)}},
	})
	if err != nil || !gain.IsZero() || !sameBps(ret, bps(0)) {
		t.Fatalf("ModifiedDietz = %v, %v, %v, want 0 gain and 0 bps", gain, fmtBps(ret), err)
	}
}

func TestModifiedDietz(t *testing.T) {
	t.Parallel()
	t1 := clock.Real{}.Now().UTC()
	t0 := t1.Add(-10 * time.Hour)
	at := func(h int) time.Time { return t0.Add(time.Duration(h) * time.Hour) }
	tests := map[string]struct {
		in       domain.DietzInput
		wantGain money.SignedMicros
		wantRet  *domain.Bps
	}{
		"no flows is the simple return": {
			in:       domain.DietzInput{Start: usd(100), End: usd(110)},
			wantGain: signed(10), wantRet: bps(1_000),
		},
		"deposit weighs by time held": {
			in: domain.DietzInput{
				Start: usd(100),
				End:   usd(160),
				Flows: []domain.Flow{{Amount: signed(50), At: at(5)}},
			},
			wantGain: signed(10),
			wantRet:  bps(800),
		},
		"cash out lowers the base": {
			in: domain.DietzInput{
				Start: usd(100),
				End:   usd(55),
				Flows: []domain.Flow{{Amount: signed(-50), At: at(5)}},
			},
			wantGain: signed(5),
			wantRet:  bps(666),
		},
		"flow at the start counts fully": {
			in:       domain.DietzInput{End: usd(110), Flows: []domain.Flow{{Amount: signed(100), At: at(0)}}},
			wantGain: signed(10), wantRet: bps(1_000),
		},
		"flow at the end has no weight": {
			in: domain.DietzInput{
				Start: usd(100),
				End:   usd(150),
				Flows: []domain.Flow{{Amount: signed(50), At: at(10)}},
			},
			wantGain: signed(0),
			wantRet:  bps(0),
		},
		"loss floors toward minus infinity": {
			in:       domain.DietzInput{Start: usd(3), End: usd(2)},
			wantGain: signed(-1), wantRet: bps(-3_334),
		},
		"empty start and no flows has no return": {
			in:       domain.DietzInput{End: usd(5)},
			wantGain: signed(5),
		},
		"cashed out below zero base has no return": {
			in:       domain.DietzInput{Start: usd(10), Flows: []domain.Flow{{Amount: signed(-30), At: at(0)}}},
			wantGain: signed(20),
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tt.in.T0, tt.in.T1 = t0, t1
			gain, ret, err := domain.ModifiedDietz(tt.in)
			if err != nil || gain != tt.wantGain || !sameBps(ret, tt.wantRet) {
				t.Fatalf(
					"ModifiedDietz = %v, %v, %v, want %v, %v",
					gain,
					fmtBps(ret),
					err,
					tt.wantGain,
					fmtBps(tt.wantRet),
				)
			}
		})
	}
}

func TestModifiedDietz_Invalid(t *testing.T) {
	t.Parallel()
	t1 := clock.Real{}.Now().UTC()
	t0 := t1.Add(-time.Hour)
	tests := map[string]domain.DietzInput{
		"empty range":     {T0: t1, T1: t1},
		"reversed range":  {T0: t1, T1: t0},
		"flow before t0":  {T0: t0, T1: t1, Flows: []domain.Flow{{At: t0.Add(-time.Nanosecond)}}},
		"flow after t1":   {T0: t0, T1: t1, Flows: []domain.Flow{{At: t1.Add(time.Nanosecond)}}},
		"gain past int64": {T0: t0, T1: t1, End: usd(math.MaxUint64)},
		"return past bps": {T0: t0, T1: t1, Start: usd(1), End: usd(math.MaxInt64)},
		"outflow overflow": {
			T0:    t0,
			T1:    t1,
			Flows: []domain.Flow{{Amount: signed(math.MinInt64), At: t0}, {Amount: signed(-1), At: t0}},
		},
	}
	for name, in := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			gain, ret, err := domain.ModifiedDietz(in)
			if errs.CodeOf(err) != errs.CodeInvalidInput || !gain.IsZero() || ret != nil {
				t.Fatalf("ModifiedDietz = %v, %v, %v, want invalid_input", gain, fmtBps(ret), err)
			}
		})
	}
}
