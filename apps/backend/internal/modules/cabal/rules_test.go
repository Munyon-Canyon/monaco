package cabal_test

import (
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
)

type rulesArgs struct {
	join, voters, threshold string
	expiry, bps             int32
}

func baseRules() rulesArgs { return rulesArgs{"open", "all", "majority", 86400, 100} }

func (a rulesArgs) build() (domain.Rules, error) {
	return domain.NewRules(a.join, a.voters, a.threshold, a.expiry, a.bps)
}

func wantCode(t *testing.T, what string, err error, code errs.Code) {
	t.Helper()
	if got := errs.CodeOf(err); err == nil || got != code {
		t.Errorf("%s: err = %v (code %s), want %s", what, err, got, code)
	}
}

func attrValue(err error, key string) string {
	for _, a := range errs.Detail(err) {
		if a.Key == key {
			return a.Value.String()
		}
	}
	return ""
}

func TestNewRules_keepsEveryAllowedValueOfEveryField(t *testing.T) {
	t.Parallel()
	with := func(edit func(*rulesArgs)) rulesArgs {
		a := baseRules()
		edit(&a)
		return a
	}
	for _, a := range []rulesArgs{
		baseRules(),
		with(func(a *rulesArgs) { a.join = "request" }),
		with(func(a *rulesArgs) { a.voters = "list" }),
		with(func(a *rulesArgs) { a.threshold = "unanimous" }),
		with(func(a *rulesArgs) { a.expiry = 3600 }),
		with(func(a *rulesArgs) { a.expiry = 604800 }),
		with(func(a *rulesArgs) { a.bps = 1 }),
		with(func(a *rulesArgs) { a.bps = 300 }),
	} {
		r, err := a.build()
		got := rulesArgs{
			string(r.JoinMode()),
			string(r.VoterMode()),
			string(r.Threshold()),
			r.ExpirySeconds(),
			r.SlippageBps(),
		}
		if err != nil || got != a {
			t.Errorf("NewRules(%+v) = %+v, %v; want the same values back", a, got, err)
		}
	}
}

func TestNewRules_refusesEachBadValueAndNamesTheField(t *testing.T) {
	t.Parallel()
	with := func(edit func(*rulesArgs)) rulesArgs {
		a := baseRules()
		edit(&a)
		return a
	}
	for _, tt := range []struct {
		name string
		args rulesArgs
		want string
	}{
		{"empty join mode", with(func(a *rulesArgs) { a.join = "" }), "join_mode"},
		{"unknown join mode", with(func(a *rulesArgs) { a.join = "invite_only" }), "join_mode"},
		{"join mode in the wrong case", with(func(a *rulesArgs) { a.join = "Open" }), "join_mode"},
		{"unknown voter mode", with(func(a *rulesArgs) { a.voters = "some" }), "voter_mode"},
		{"unknown threshold", with(func(a *rulesArgs) { a.threshold = "plurality" }), "threshold"},
		{"zero expiry", with(func(a *rulesArgs) { a.expiry = 0 }), "proposal_expiry_seconds"},
		{"two hour expiry", with(func(a *rulesArgs) { a.expiry = 7200 }), "proposal_expiry_seconds"},
		{"negative expiry", with(func(a *rulesArgs) { a.expiry = -3600 }), "proposal_expiry_seconds"},
		{"zero slippage", with(func(a *rulesArgs) { a.bps = 0 }), "slippage_bps"},
		{"slippage over the cap", with(func(a *rulesArgs) { a.bps = 301 }), "slippage_bps"},
		{"negative slippage", with(func(a *rulesArgs) { a.bps = -1 }), "slippage_bps"},
		{"two bad fields", with(func(a *rulesArgs) { a.join, a.bps = "x", 0 }), "join_mode,slippage_bps"},
		{"every field bad", rulesArgs{"x", "x", "x", 1, 0}, "join_mode,voter_mode,threshold,proposal_expiry_seconds,slippage_bps"},
	} {
		r, err := tt.args.build()
		wantCode(t, tt.name, err, errs.CodeInvalidInput)
		if got := attrValue(err, "fields"); got != tt.want || r != (domain.Rules{}) {
			t.Errorf("%s: fields = %q and rules %+v, want %q and the zero value", tt.name, got, r, tt.want)
		}
	}
}

func wantBadFields(a rulesArgs) string {
	var bad []string
	if !slices.Contains([]string{"open", "request"}, a.join) {
		bad = append(bad, "join_mode")
	}
	if !slices.Contains([]string{"all", "list"}, a.voters) {
		bad = append(bad, "voter_mode")
	}
	if !slices.Contains([]string{"majority", "unanimous"}, a.threshold) {
		bad = append(bad, "threshold")
	}
	if !slices.Contains([]int32{3600, 86400, 604800}, a.expiry) {
		bad = append(bad, "proposal_expiry_seconds")
	}
	if a.bps < 1 || a.bps > 300 {
		bad = append(bad, "slippage_bps")
	}
	return strings.Join(bad, ",")
}

func TestNewRules_acceptsExactlyTheAllowedValues(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		a := rulesArgs{
			join:   rapid.OneOf(rapid.SampledFrom([]string{"open", "request"}), rapid.String()).Draw(t, "join"),
			voters: rapid.OneOf(rapid.SampledFrom([]string{"all", "list"}), rapid.String()).Draw(t, "voters"),
			threshold: rapid.OneOf(rapid.SampledFrom([]string{"majority", "unanimous"}), rapid.String()).
				Draw(t, "threshold"),
			expiry: rapid.OneOf(rapid.SampledFrom([]int32{3600, 86400, 604800}), rapid.Int32()).Draw(t, "expiry"),
			bps:    rapid.OneOf(rapid.Int32Range(1, 300), rapid.Int32()).Draw(t, "bps"),
		}
		r, err := a.build()
		if want := wantBadFields(a); want != "" {
			if errs.CodeOf(err) != errs.CodeInvalidInput || err == nil || attrValue(err, "fields") != want {
				t.Fatalf("NewRules(%+v) = %v, want invalid_input naming %q", a, err, want)
			}
			return
		}
		if err != nil || r.JoinMode() != domain.JoinMode(a.join) || r.SlippageBps() != a.bps ||
			r.ExpirySeconds() != a.expiry {
			t.Fatalf("NewRules(%+v) = %+v, %v; want it accepted unchanged", a, r, err)
		}
	})
}

func TestVoterFor_theCreatorAlwaysVotesAndMembersVoteOnlyWhenEveryoneDoes(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		voters string
		role   domain.Role
		want   bool
	}{
		{"all", domain.RoleCreator, true},
		{"all", domain.RoleMember, true},
		{"list", domain.RoleCreator, true},
		{"list", domain.RoleMember, false},
	} {
		a := baseRules()
		a.voters = tt.voters
		r, err := a.build()
		if err != nil {
			t.Fatal(err)
		}
		if got := domain.VoterFor(r, tt.role); got != tt.want {
			t.Errorf("VoterFor(%s cabal, %s) = %t, want %t", tt.voters, tt.role, got, tt.want)
		}
	}
}
