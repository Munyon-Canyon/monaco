package domain_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func bps(v int32) *int32 { return &v }

func newUser(seed uint64) ids.UserID { return ids.UserIDFrom(testkit.NewIDs(seed).NewV7()) }

func noErr(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func isErr(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func eq(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

type ruleBuilder func() (domain.RuleChange, error)

func TestRuleChange_constructorsValidate(t *testing.T) {
	t.Parallel()
	user := newUser(3)
	valid := []ruleBuilder{
		func() (domain.RuleChange, error) { return domain.ShowSidesChange(false, true), nil },
		func() (domain.RuleChange, error) { return domain.MaxTradeBpsChange(nil, bps(2500)) },
		func() (domain.RuleChange, error) { return domain.MaxTradeBpsChange(bps(100), nil) },
		func() (domain.RuleChange, error) { return domain.ThresholdChange("majority", "unanimous") },
		func() (domain.RuleChange, error) { return domain.ExpiryChange(3600, 604800) },
		func() (domain.RuleChange, error) { return domain.JoinModeChange("open", "request") },
		func() (domain.RuleChange, error) { return domain.VoterChange(user, false, true), nil },
		func() (domain.RuleChange, error) { return domain.AdminChange(user, true, false), nil },
	}
	for _, build := range valid {
		change, err := build()
		noErr(t, err)
		if change.Rule() == "" {
			t.Fatal("empty rule")
		}
	}
	invalid := []ruleBuilder{
		func() (domain.RuleChange, error) { return domain.MaxTradeBpsChange(nil, bps(50)) },
		func() (domain.RuleChange, error) { return domain.MaxTradeBpsChange(bps(150), nil) },
		func() (domain.RuleChange, error) { return domain.MaxTradeBpsChange(nil, bps(10100)) },
		func() (domain.RuleChange, error) { return domain.ThresholdChange("majority", "half") },
		func() (domain.RuleChange, error) { return domain.ThresholdChange("half", "majority") },
		func() (domain.RuleChange, error) { return domain.ExpiryChange(3600, 7200) },
		func() (domain.RuleChange, error) { return domain.ExpiryChange(7200, 3600) },
		func() (domain.RuleChange, error) { return domain.JoinModeChange("open", "closed") },
		func() (domain.RuleChange, error) { return domain.JoinModeChange("closed", "open") },
	}
	for _, build := range invalid {
		_, err := build()
		eq(t, errs.CodeOf(err), errs.CodeInvalidInput)
	}
}

func TestRuleChange_accessors(t *testing.T) {
	t.Parallel()
	user := newUser(3)
	cabalWide := domain.ShowSidesChange(false, true)
	eq(t, domain.RuleShowSides, cabalWide.Rule())
	eq(t, false, cabalWide.From())
	eq(t, true, cabalWide.To())
	if cabalWide.UserID() != nil {
		t.Fatal("cabal-wide rule has a user")
	}
	perMember := domain.VoterChange(user, true, false)
	eq(t, user, *perMember.UserID())
}

func TestRuleChange_jsonRoundTrip(t *testing.T) {
	t.Parallel()
	user := newUser(3)
	trade, err := domain.MaxTradeBpsChange(nil, bps(500))
	noErr(t, err)
	threshold, err := domain.ThresholdChange("majority", "unanimous")
	noErr(t, err)
	expiry, err := domain.ExpiryChange(3600, 86400)
	noErr(t, err)
	join, err := domain.JoinModeChange("request", "open")
	noErr(t, err)
	for _, change := range []domain.RuleChange{
		domain.ShowSidesChange(false, true), trade, threshold, expiry, join,
		domain.VoterChange(user, false, true), domain.AdminChange(user, true, false), domain.CreatorChange(user),
	} {
		raw, err := json.Marshal(change)
		noErr(t, err)
		if !strings.Contains(string(raw), `"v":1`) {
			t.Fatalf("no version in %s", raw)
		}
		var back domain.RuleChange
		noErr(t, json.Unmarshal(raw, &back))
		eq(t, change, back)
	}
}

func TestRuleChange_jsonRejectsBadInput(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"not json":        `{`,
		"wrong shape":     `[]`,
		"wrong version":   `{"v":2,"rule":"show_sides","from":false,"to":true}`,
		"unknown rule":    `{"v":1,"rule":"nope","from":false,"to":true}`,
		"bad bool":        `{"v":1,"rule":"show_sides","from":"x","to":true}`,
		"bad bps":         `{"v":1,"rule":"max_trade_bps","from":null,"to":"x"}`,
		"out of range":    `{"v":1,"rule":"max_trade_bps","from":null,"to":50}`,
		"bad expiry":      `{"v":1,"rule":"proposal_expiry_seconds","from":3600,"to":1}`,
		"bad expiry typ":  `{"v":1,"rule":"proposal_expiry_seconds","from":"a","to":1}`,
		"bad threshold":   `{"v":1,"rule":"threshold","from":"majority","to":"half"}`,
		"bad join":        `{"v":1,"rule":"join_mode","from":"open","to":"closed"}`,
		"bad join type":   `{"v":1,"rule":"join_mode","from":1,"to":"open"}`,
		"voter no user":   `{"v":1,"rule":"voter","from":false,"to":true}`,
		"admin no user":   `{"v":1,"rule":"admin","from":false,"to":true}`,
		"creator no user": `{"v":1,"rule":"creator","from":false,"to":true}`,
		"creator off":     `{"v":1,"rule":"creator","from":true,"to":false,"user_id":"00000000-0000-0000-0000-000000000003"}`,
	}
	for name, raw := range cases {
		var change domain.RuleChange
		if json.Unmarshal([]byte(raw), &change) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestRuleChange_pureChecks(t *testing.T) {
	t.Parallel()
	user := newUser(1)
	creator := newUser(2)
	isErr(t, domain.ShowSidesChange(true, true).Unchanged(), domain.ErrRuleUnchanged)
	noErr(t, domain.ShowSidesChange(false, true).Unchanged())
	same, err := domain.MaxTradeBpsChange(nil, nil)
	noErr(t, err)
	isErr(t, same.Unchanged(), domain.ErrRuleUnchanged)

	removeAdmin := domain.AdminChange(user, true, false)
	isErr(t, removeAdmin.KeepsAnAdmin(1), domain.ErrLastAdmin)
	noErr(t, removeAdmin.KeepsAnAdmin(2))
	noErr(t, domain.AdminChange(user, false, true).KeepsAnAdmin(1))
	noErr(t, domain.ShowSidesChange(true, false).KeepsAnAdmin(1))

	makeAdmin := domain.AdminChange(user, false, true)
	isErr(t, makeAdmin.AdminIsVoter(false), domain.ErrAdminNotVoter)
	noErr(t, makeAdmin.AdminIsVoter(true))
	noErr(t, removeAdmin.AdminIsVoter(false))

	isErr(t, domain.VoterChange(creator, true, false).NotCreator(creator), domain.ErrTargetsCreator)
	noErr(t, domain.VoterChange(user, true, false).NotCreator(creator))
	noErr(t, domain.ShowSidesChange(false, true).NotCreator(creator))
}

func TestRuleChange_passCreatorChecks(t *testing.T) {
	t.Parallel()
	creator, admin, member := newUser(1), newUser(2), newUser(3)
	pass := domain.CreatorChange(admin)
	eq(t, domain.RuleCreator, pass.Rule())
	noErr(t, pass.Unchanged())
	noErr(t, pass.ProposedByCreator(creator, creator))
	isErr(t, pass.ProposedByCreator(admin, creator), domain.ErrNotCreator)
	noErr(t, domain.ShowSidesChange(false, true).ProposedByCreator(admin, creator))
	noErr(t, pass.TargetIsAdmin([]ids.UserID{creator, admin}))
	isErr(t, pass.TargetIsAdmin([]ids.UserID{creator}), domain.ErrNotAnAdmin)
	isErr(t, domain.CreatorChange(member).TargetIsAdmin([]ids.UserID{creator, admin}), domain.ErrNotAnAdmin)
	noErr(t, domain.AdminChange(member, false, true).TargetIsAdmin(nil))
	isErr(t, domain.CreatorChange(creator).NotCreator(creator), domain.ErrTargetsCreator)
}

func TestParseKind_rule(t *testing.T) {
	t.Parallel()
	kind, err := domain.ParseKind("rule")
	noErr(t, err)
	eq(t, domain.KindRule, kind)
}

func TestNewTrade_refusesARuleKind(t *testing.T) {
	t.Parallel()
	_, err := domain.NewTrade(domain.Trade{Kind: domain.KindRule, Symbol: "NVDAx"})
	if err == nil {
		t.Fatal("NewTrade(rule) = nil, want the kind refused")
	}
}
