package market_test

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
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

func TestTradable_uncheckedChainFactsKeepAnAssetOutWhateverTheOverride(t *testing.T) {
	t.Parallel()
	for _, override := range []domain.Override{domain.OverrideAuto, domain.OverrideOn, domain.OverrideOff} {
		a := marketfake.AAPLx()
		a.ChainChecked, a.Override = false, override
		if a.Tradable() {
			t.Fatalf("unchecked AAPLx with override %s is tradable, want untradable until the chain check", override)
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

func TestCrossed_reportsEveryThresholdTheBasisPointChangeHasReached(t *testing.T) {
	t.Parallel()
	usd := money.MicrosFromUint64
	for _, tc := range []struct {
		name string
		prev money.Micros
		mark money.Micros
		bps  int64
		want []domain.ThresholdBps
	}{
		{name: "zero reference", mark: usd(100)},
		{name: "unchanged", prev: usd(200_000_000), mark: usd(200_000_000)},
		{name: "just under five up", prev: usd(100_000), mark: usd(104_999), bps: 499},
		{name: "five up", prev: usd(100_000), mark: usd(105_000), bps: 500, want: []domain.ThresholdBps{500}},
		{
			name: "ten up", prev: usd(200_000_000), mark: usd(220_000_000), bps: 1000,
			want: []domain.ThresholdBps{500, 1000},
		},
		{name: "just under five down", prev: usd(100_000), mark: usd(95_001), bps: -499},
		{name: "five down", prev: usd(100_000), mark: usd(95_000), bps: -500, want: []domain.ThresholdBps{-500}},
		{
			name: "ten down", prev: usd(200_000_000), mark: usd(180_000_000), bps: -1000,
			want: []domain.ThresholdBps{-500, -1000},
		},
		{
			name: "toward zero on a fraction", prev: usd(100_000), mark: usd(105_009), bps: 500,
			want: []domain.ThresholdBps{500},
		},
		{
			name: "huge rise", prev: usd(1), mark: usd(math.MaxUint64), bps: math.MaxInt64,
			want: []domain.ThresholdBps{500, 1000},
		},
		{
			name: "huge fall", prev: usd(math.MaxUint64), mark: usd(1), bps: -9999,
			want: []domain.ThresholdBps{-500, -1000},
		},
	} {
		got := domain.Crossed(tc.prev, tc.mark)
		if domain.ChangeBps(tc.prev, tc.mark) != tc.bps || !slices.Equal(got, tc.want) {
			t.Fatalf("%s: Crossed = %v change %d, want %v and %d",
				tc.name, got, domain.ChangeBps(tc.prev, tc.mark), tc.want, tc.bps)
		}
	}
}

func TestCleanName_stripsATrailingIssuerSuffixWithoutLowercasing(t *testing.T) {
	t.Parallel()
	for name, want := range map[string]string{
		"Apple xStock":    "Apple",
		"  Tesla XSTOCK ": "Tesla",
		"SpaceX":          "SpaceX",
	} {
		if got := domain.CleanName(name); got != want {
			t.Fatalf("CleanName(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestFirstAcceptedOnDay_usesTheFirstConfirmedSampleOfThatDay(t *testing.T) {
	t.Parallel()
	day := clock.Real{}.Now().UTC().Truncate(24 * time.Hour)
	usd := money.MicrosFromUint64
	sample := func(micros uint64, at time.Time) domain.Sample {
		return domain.Sample{Micros: usd(micros), ObservedAt: at}
	}
	prior := sample(200_000_000, day.Add(-time.Hour))
	older := sample(200_000_000, day.Add(-2*time.Hour))
	oldest := sample(200_000_000, day.Add(-3*time.Hour))
	noon := sample(220_000_000, day.Add(12*time.Hour))
	spike := sample(250_000_000, day.Add(time.Hour))
	if _, ok := domain.FirstAcceptedOnDay(nil, day); ok {
		t.Fatal("FirstAcceptedOnDay(nil) accepted a sample")
	}
	if _, ok := domain.FirstAcceptedOnDay([]domain.Sample{prior}, day); ok {
		t.Fatal("FirstAcceptedOnDay accepted a sample from before the day")
	}
	if _, ok := domain.FirstAcceptedOnDay([]domain.Sample{prior, spike}, day); ok {
		t.Fatal("FirstAcceptedOnDay accepted an unconfirmed spike")
	}
	got, ok := domain.FirstAcceptedOnDay([]domain.Sample{oldest, older, prior, noon}, day)
	if !ok || got != noon {
		t.Fatalf("FirstAcceptedOnDay = %+v, %v, want the noon sample", got, ok)
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

func TestUIMultiplierAt_switchesToTheScheduledMultiplierAtItsEffectiveSecond(t *testing.T) {
	t.Parallel()
	step := clock.Real{}.Now().Truncate(time.Second)
	a := marketfake.AAPLx()
	a.UIMultiplier = domain.Multiplier{Num: 3, Den: 2}
	if got := a.UIMultiplierAt(step); got != a.UIMultiplier {
		t.Fatalf("with nothing scheduled: UIMultiplierAt = %+v, want the current %+v", got, a.UIMultiplier)
	}
	a.NextUIMultiplier = domain.MultiplierStep{To: domain.Multiplier{Num: 2, Den: 1}, At: step}
	for at, want := range map[time.Time]domain.Multiplier{
		step.Add(-time.Nanosecond):    a.UIMultiplier,
		step:                          a.NextUIMultiplier.To,
		step.Add(30 * 24 * time.Hour): a.NextUIMultiplier.To,
	} {
		if got := a.UIMultiplierAt(at); got != want {
			t.Fatalf("UIMultiplierAt(%s) with 2/1 from %s = %+v, want %+v", at, step, got, want)
		}
	}
}
