package app

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type preIPORead struct {
	profiles    []sqlc.PreIpoProfilesRow
	profilesErr error
	rounds      []sqlc.PreIpoRound
	roundsErr   error
}

func (r preIPORead) PreIpoProfiles(context.Context, []string) ([]sqlc.PreIpoProfilesRow, error) {
	return r.profiles, r.profilesErr
}

func (r preIPORead) PreIpoRounds(context.Context, []string) ([]sqlc.PreIpoRound, error) {
	return r.rounds, r.roundsErr
}

func profileRow(mutate func(*sqlc.PreIpoProfilesRow)) sqlc.PreIpoProfilesRow {
	row := sqlc.PreIpoProfilesRow{
		CompanyKey: "acme", EstimateMicros: 185_000_000, ValuationMicros: "1", MinOrderMicros: 10_000_000,
		SellWindow: "any_time",
	}
	mutate(&row)
	return row
}

func badProfile(mutate func(*sqlc.PreIpoProfilesRow)) preIPORead {
	return preIPORead{profiles: []sqlc.PreIpoProfilesRow{profileRow(mutate)}}
}

func TestLoadProfiles_failsOnAReadErrorOrAnUnreadableRow(t *testing.T) {
	t.Parallel()
	assets := []domain.Asset{{Kind: domain.KindPreIPO, CompanyKey: "acme"}}
	cases := map[string]preIPORead{
		"read error":        {profilesErr: errs.New(errs.CodeUpstreamUnavailable, "test")},
		"zero estimate":     badProfile(func(r *sqlc.PreIpoProfilesRow) { r.EstimateMicros = 0 }),
		"zero minimum":      badProfile(func(r *sqlc.PreIpoProfilesRow) { r.MinOrderMicros = 0 }),
		"unknown sell mode": badProfile(func(r *sqlc.PreIpoProfilesRow) { r.SellWindow = "daily" }),
	}
	for name, read := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got, err := loadProfiles(t.Context(), read, assets); err == nil {
				t.Fatalf("profiles = %v, want an error", got)
			}
		})
	}
}

func TestLoadProfiles_skipsEquitiesAndAnAbsentReader(t *testing.T) {
	t.Parallel()
	failing := preIPORead{profilesErr: errs.New(errs.CodeInternal, "test")}
	equity := []domain.Asset{{Kind: domain.KindEquity, CompanyKey: "acme"}}
	if got, err := loadProfiles(t.Context(), failing, equity); err != nil || len(got) != 0 {
		t.Fatalf("equity profiles = %v, %v, want none and no query", got, err)
	}
	preIPO := []domain.Asset{{Kind: domain.KindPreIPO, CompanyKey: "acme"}}
	if got, err := loadProfiles(t.Context(), nil, preIPO); err != nil || len(got) != 0 {
		t.Fatalf("profiles without a reader = %v, %v, want none", got, err)
	}
}

func TestLoadRounds_failsOnAReadErrorOrANonPositivePrice(t *testing.T) {
	t.Parallel()
	cases := map[string]preIPORead{
		"read error":     {roundsErr: errs.New(errs.CodeUpstreamUnavailable, "test")},
		"zero round":     {rounds: []sqlc.PreIpoRound{{CompanyKey: "acme", PriceMicros: 0}}},
		"negative round": {rounds: []sqlc.PreIpoRound{{CompanyKey: "acme", PriceMicros: -1}}},
	}
	for name, read := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got, err := loadRounds(t.Context(), read, "acme"); err == nil {
				t.Fatalf("rounds = %v, want an error", got)
			}
		})
	}
}

func TestDetail_reportsAProfileOrRoundsReadFailureOfAPreIPOCompany(t *testing.T) {
	t.Parallel()
	asset := appleRow(t, "pre_ipo")
	boom := errs.New(errs.CodeUpstreamUnavailable, "market.test")
	profile := profileRow(func(r *sqlc.PreIpoProfilesRow) { r.CompanyKey = "apple" })
	cases := map[string]preIPORead{
		"profiles": {profilesErr: boom},
		"rounds":   {profiles: []sqlc.PreIpoProfilesRow{profile}, roundsErr: boom},
	}
	for name, read := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			clk := testkit.NewClock(time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC))
			list := &ListAssets{read: reader{}, volumes: reader{}, profiles: read, clock: clk}
			d := &Detail{list: list, read: detailRead{asset: func() (sqlc.Asset, error) { return asset, nil }}}
			if _, err := d.Handle(t.Context(), "AAPLx"); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
				t.Fatalf("detail err = %v, want upstream_unavailable", err)
			}
		})
	}
}
