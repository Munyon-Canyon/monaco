package market_test

import (
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestParse_acceptsEachKnownValue(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"xstocks", "tessera", "prestocks"} {
		if got, err := domain.ParseIssuer(raw); err != nil || string(got) != raw {
			t.Fatalf("ParseIssuer(%q) = %q, %v", raw, got, err)
		}
	}
	for _, raw := range []string{"equity", "pre_ipo"} {
		if got, err := domain.ParseKind(raw); err != nil || string(got) != raw {
			t.Fatalf("ParseKind(%q) = %q, %v", raw, got, err)
		}
	}
	for _, raw := range []string{"auto", "on", "off"} {
		if got, err := domain.ParseOverride(raw); err != nil || string(got) != raw {
			t.Fatalf("ParseOverride(%q) = %q, %v", raw, got, err)
		}
	}
}

func TestParse_rejectsUnknownValues(t *testing.T) {
	t.Parallel()
	for name, err := range map[string]error{
		"issuer":   second(domain.ParseIssuer("backed")),
		"kind":     second(domain.ParseKind("bond")),
		"override": second(domain.ParseOverride("maybe")),
	} {
		if errs.CodeOf(err) != errs.CodeInvalidInput {
			t.Fatalf("unknown %s err = %v, want invalid_input", name, err)
		}
	}
}

func TestParseMint_acceptsASolanaAddressOnly(t *testing.T) {
	t.Parallel()
	_, err := domain.ParseMint("0x9d275685dc284c8eb1c79f6aba7a63dc75ec890a")
	if errs.CodeOf(err) != errs.CodeInvalidAddress {
		t.Fatalf("ParseMint(evm address) err = %v, want invalid_address", err)
	}
	mint, err := domain.ParseMint("XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp")
	if err != nil || string(mint.Address()) != mint.String() {
		t.Fatalf("ParseMint = %v, %v", mint, err)
	}
}

func second[T any](_ T, err error) error { return err }

func TestTradableOverride_WinsOverIssuer(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		issuer   bool
		override domain.Override
		want     bool
	}{
		{issuer: true, override: domain.OverrideOff, want: false},
		{issuer: false, override: domain.OverrideOn, want: true},
		{issuer: true, override: domain.OverrideOn, want: true},
		{issuer: false, override: domain.OverrideOff, want: false},
	} {
		a := marketfake.AAPLx()
		a.IssuerTradable, a.Override = tc.issuer, tc.override
		if a.Tradable() != tc.want {
			t.Fatalf("issuer %v override %s: Tradable = %v, want %v", tc.issuer, tc.override, a.Tradable(), tc.want)
		}
	}
}

func TestTradableOverride_AutoFollowsTheIssuer(t *testing.T) {
	t.Parallel()
	for _, issuer := range []bool{true, false} {
		a := marketfake.AAPLx()
		a.IssuerTradable, a.Override = issuer, domain.OverrideAuto
		if a.Tradable() != issuer {
			t.Fatalf("auto with issuer %v: Tradable = %v", issuer, a.Tradable())
		}
	}
}

func TestCompanyKey_lowercasesAndStripsTheIssuerSuffix(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]string{
		"Apple xStock":                       "apple",
		"  Alphabet Class A xStock ":         "alphabet class a",
		"SpaceX":                             "spacex",
		"JPMorgan Ultra-Short Income xStock": "jpmorgan ultra-short income",
	} {
		if got := domain.CompanyKey(name); got != want {
			t.Fatalf("CompanyKey(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestPopularRank_ranksPinnedSymbolsFromOneAndLeavesTheRestAtZero(t *testing.T) {
	t.Parallel()
	for symbol, want := range map[string]int16{"AAPLx": 1, "TSLAx": 7, "ORCLx": 17, "XRXx": 0, "aaplx": 0} {
		if got := domain.PopularRank(symbol); got != want {
			t.Fatalf("PopularRank(%q) = %d, want %d", symbol, got, want)
		}
	}
}

func TestNewMultiplier_takesAPositiveFractionThatFitsTheColumns(t *testing.T) {
	t.Parallel()
	got, err := domain.NewMultiplier(math.MaxInt64, 3)
	if err != nil || got != (domain.Multiplier{Num: math.MaxInt64, Den: 3}) {
		t.Fatalf("NewMultiplier(MaxInt64, 3) = %+v, %v, want it kept whole", got, err)
	}
	for _, bad := range [][2]uint64{{0, 1}, {1, 0}, {math.MaxInt64 + 1, 1}, {1, math.MaxInt64 + 1}} {
		if _, err := domain.NewMultiplier(bad[0], bad[1]); errs.CodeOf(err) != errs.CodeDecodeFailed {
			t.Fatalf("NewMultiplier(%d, %d) err = %v, want decode_failed", bad[0], bad[1], err)
		}
	}
}
