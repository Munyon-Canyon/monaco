package governance_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type ruleWorld struct {
	voteDB
	cabal   ids.CabalID
	members []ids.UserID
	fake    *fakes.Cabal
}

type ruleWorldOpts struct {
	status    cabal.Status
	threshold cabal.Threshold
	roles     map[int]cabal.Role
	nonVoters map[int]bool
	size      int
}

func newRuleWorld(t *testing.T, o ruleWorldOpts) *ruleWorld {
	t.Helper()
	if o.size == 0 {
		o.size = 8
	}
	if o.status == "" {
		o.status = cabal.StatusActive
	}
	if o.threshold == "" {
		o.threshold = cabal.ThresholdMajority
	}
	d := newVoteDB(t)
	w := &ruleWorld{voteDB: d, cabal: ids.CabalIDFrom(d.ids.NewV7())}
	rows := make([]fakes.CabalMember, 0, o.size)
	for i := range o.size {
		user := ids.UserIDFrom(w.ids.NewV7())
		w.members = append(w.members, user)
		role := cabal.RoleMember
		if i == 0 {
			role = cabal.RoleCreator
		}
		if r, ok := o.roles[i]; ok {
			role = r
		}
		rows = append(rows, fakes.CabalMember{CabalID: w.cabal, Member: cabal.MemberView{
			UserID: user, Role: role, CanVote: !o.nonVoters[i], JoinedAt: w.now.Add(time.Duration(i) * time.Minute),
		}})
	}
	w.fake = fakes.NewCabal([]fakes.CabalSeed{{
		View: cabal.View{ID: w.cabal, Name: "Pot", CreatorID: w.members[0], Status: o.status},
		Rules: cabal.Rules{
			JoinMode: cabal.JoinOpen, VoterMode: cabal.VotersAll, Threshold: o.threshold,
			ProposalExpiry: 24 * time.Hour, SlippageBps: 100,
		},
	}}, rows)
	return w
}

func (w *ruleWorld) handler() *app.ProposeRuleChangeHandler {
	return app.NewProposeRuleChangeHandler(w.uow, w.ids, w.clk, w.fake, app.NoHints{})
}

func (w *ruleWorld) propose(by int, target app.RuleTarget) (ids.ProposalID, error) {
	ctx := observability.WithActor(t0(), "user:"+w.members[by].UUID().String())
	return w.handler().Handle(ctx, app.ProposeRuleChange{
		CabalID: w.cabal, ProposerID: w.members[by], Target: target, Thesis: "why",
	})
}

const adminRole cabal.Role = "admin"

func t0() context.Context { return context.Background() }

func yesTo(v bool) app.RuleTarget { return app.RuleTarget{Rule: domain.RuleShowSides, Bool: &v} }

func (w *ruleWorld) member(rule domain.Rule, to bool, who int) app.RuleTarget {
	return app.RuleTarget{Rule: rule, Bool: &to, UserID: &w.members[who]}
}

func (w *ruleWorld) mustPropose(t *testing.T, target app.RuleTarget) ids.ProposalID {
	t.Helper()
	id, err := w.propose(1, target)
	if err != nil {
		t.Fatalf("propose = %v", err)
	}
	return id
}

type storedRule struct {
	kind, status, threshold string
	change                  []byte
	symbol, usdc, quote     *string
	voters                  int
}

func (w *ruleWorld) stored(t *testing.T, id ids.ProposalID) storedRule {
	t.Helper()
	var r storedRule
	err := w.pool.QueryRow(t.Context(), `SELECT p.kind, p.status, p.threshold, p.rule_change, p.symbol,
		p.usdc_micros::text, p.quote_out_amount::text, (SELECT count(*) FROM proposal_voters WHERE proposal_id = p.id)
		FROM proposals p WHERE p.id = $1`, id.UUID()).
		Scan(&r.kind, &r.status, &r.threshold, &r.change, &r.symbol, &r.usdc, &r.quote, &r.voters)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestProposeRuleChange_storesAnOpenRuleProposalWithTheVoterSnapshot(t *testing.T) {
	t.Parallel()
	w := newRuleWorld(t, ruleWorldOpts{nonVoters: map[int]bool{7: true}})
	got := w.stored(t, w.mustPropose(t, yesTo(true)))
	if got.kind != "rule" || got.status != "open" || got.threshold != "majority" || got.voters != 7 {
		t.Fatalf("row = %+v, want rule open majority with 7 voters", got)
	}
	if got.symbol != nil || got.usdc != nil || got.quote != nil {
		t.Fatalf("trade fields = %v %v %v, want null", got.symbol, got.usdc, got.quote)
	}
	if string(got.change) != `{"v": 1, "to": true, "from": false, "rule": "show_sides"}` {
		t.Fatalf("rule_change = %s", got.change)
	}
}

func TestProposeRuleChange_emitsProposalCreatedWithTheRuleKind(t *testing.T) {
	t.Parallel()
	w := newRuleWorld(t, ruleWorldOpts{nonVoters: map[int]bool{7: true}})
	id := w.mustPropose(t, yesTo(true))
	raws := w.payloads(t, id.UUID(), events.TypeProposalCreated)
	var created events.ProposalCreated
	if len(raws) != 1 || json.Unmarshal(raws[0], &created) != nil {
		t.Fatalf("proposal.created payloads = %s, want one", raws)
	}
	if created.Kind != "rule" || created.Symbol != "" || created.USDCMicros.Uint64() != 0 || created.VoterCount != 7 ||
		created.ProposerID != w.members[1].UUID() {
		t.Fatalf("proposal.created = %+v", created)
	}
}

func TestProposeRuleChange_aSecondOpenProposalForTheSameRuleIsRefused(t *testing.T) {
	t.Parallel()
	w := newRuleWorld(t, ruleWorldOpts{})
	w.mustPropose(t, yesTo(true))
	_, err := w.propose(2, yesTo(true))
	if errs.CodeOf(err) != errs.CodeRuleChangeOpen {
		t.Fatalf("second propose = %v, want rule_change_open", err)
	}
	if _, err := w.propose(2, w.member(domain.RuleVoter, false, 3)); err != nil {
		t.Fatalf("a different rule = %v, want ok", err)
	}
}

func TestProposeRuleChange_twoRacingRequestsLeaveOneOpen(t *testing.T) {
	t.Parallel()
	w := newRuleWorld(t, ruleWorldOpts{})
	start := make(chan struct{})
	var wg sync.WaitGroup
	var results [2]error
	for i := range results {
		wg.Go(func() {
			<-start
			_, results[i] = w.propose(i+1, yesTo(true))
		})
	}
	close(start)
	wg.Wait()
	var ok, open int
	for _, err := range results {
		switch {
		case err == nil:
			ok++
		case errs.CodeOf(err) == errs.CodeRuleChangeOpen:
			open++
		}
	}
	if ok != 1 || open != 1 {
		t.Fatalf("results = %v, want one created and one rule_change_open", results)
	}
}

func intTo(rule domain.Rule, v int32) app.RuleTarget { return app.RuleTarget{Rule: rule, Int: &v} }

func textTo(rule domain.Rule, v string) app.RuleTarget { return app.RuleTarget{Rule: rule, Text: &v} }

func TestProposeRuleChange_acceptsEveryRule(t *testing.T) {
	t.Parallel()
	w := newRuleWorld(t, ruleWorldOpts{roles: map[int]cabal.Role{1: adminRole}})
	for name, target := range map[string]app.RuleTarget{
		"max_trade_bps": intTo(domain.RuleMaxTradeBps, 500),
		"expiry":        intTo(domain.RuleExpiry, 3600),
		"threshold":     textTo(domain.RuleThreshold, "unanimous"),
		"join_mode":     textTo(domain.RuleJoinMode, "request"),
		"admin":         w.member(domain.RuleAdmin, true, 2),
		"voter":         w.member(domain.RuleVoter, false, 3),
	} {
		if _, err := w.propose(1, target); err != nil {
			t.Errorf("%s: propose = %v, want ok", name, err)
		}
	}
}

func TestProposeRuleChange_refusals(t *testing.T) {
	t.Parallel()
	soleAdmin := map[int]cabal.Role{0: cabal.RoleMember, 1: adminRole}
	cases := []struct {
		name   string
		opts   ruleWorldOpts
		by     int
		target func(w *ruleWorld) app.RuleTarget
		want   errs.Code
	}{
		{
			"unchanged show_sides",
			ruleWorldOpts{},
			1,
			func(*ruleWorld) app.RuleTarget { return yesTo(false) },
			errs.CodeRuleUnchanged,
		},
		{"unchanged threshold", ruleWorldOpts{}, 1, func(*ruleWorld) app.RuleTarget {
			return textTo(domain.RuleThreshold, "majority")
		}, errs.CodeRuleUnchanged},
		{"non-voter", ruleWorldOpts{nonVoters: map[int]bool{1: true}}, 1, func(*ruleWorld) app.RuleTarget {
			return yesTo(true)
		}, errs.CodeNotAVoter},
		{"banned", ruleWorldOpts{status: cabal.StatusBanned}, 1, func(*ruleWorld) app.RuleTarget {
			return yesTo(true)
		}, errs.CodeCabalBanned},
		{"bad threshold rule", ruleWorldOpts{threshold: "bad"}, 1, func(*ruleWorld) app.RuleTarget {
			return yesTo(true)
		}, errs.CodeDecodeFailed},
		{
			"no voters",
			ruleWorldOpts{
				nonVoters: map[int]bool{0: true, 1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true},
			},
			1,
			func(*ruleWorld) app.RuleTarget { return yesTo(true) },
			errs.CodeInvalidInput,
		},
		{"last admin", ruleWorldOpts{roles: soleAdmin}, 2, func(w *ruleWorld) app.RuleTarget {
			return w.member(domain.RuleAdmin, false, 1)
		}, errs.CodeInvalidInput},
		{
			"admin who cannot vote",
			ruleWorldOpts{nonVoters: map[int]bool{3: true}},
			1,
			func(w *ruleWorld) app.RuleTarget {
				return w.member(domain.RuleAdmin, true, 3)
			},
			errs.CodeInvalidInput,
		},
		{"targets the creator", ruleWorldOpts{}, 1, func(w *ruleWorld) app.RuleTarget {
			return w.member(domain.RuleVoter, false, 0)
		}, errs.CodeInvalidInput},
		{"target is not a member", ruleWorldOpts{}, 1, func(w *ruleWorld) app.RuleTarget {
			other := ids.UserIDFrom(w.ids.NewV7())
			t := true
			return app.RuleTarget{Rule: domain.RuleVoter, Bool: &t, UserID: &other}
		}, errs.CodeInvalidInput},
		{"member rule without a user", ruleWorldOpts{}, 1, func(*ruleWorld) app.RuleTarget {
			t := true
			return app.RuleTarget{Rule: domain.RuleVoter, Bool: &t}
		}, errs.CodeInvalidInput},
		{"member rule without a value", ruleWorldOpts{}, 1, func(w *ruleWorld) app.RuleTarget {
			return app.RuleTarget{Rule: domain.RuleAdmin, UserID: &w.members[2]}
		}, errs.CodeInvalidInput},
		{"show_sides without a value", ruleWorldOpts{}, 1, func(*ruleWorld) app.RuleTarget {
			return app.RuleTarget{Rule: domain.RuleShowSides}
		}, errs.CodeInvalidInput},
		{"threshold without a value", ruleWorldOpts{}, 1, func(*ruleWorld) app.RuleTarget {
			return app.RuleTarget{Rule: domain.RuleThreshold}
		}, errs.CodeInvalidInput},
		{"join_mode without a value", ruleWorldOpts{}, 1, func(*ruleWorld) app.RuleTarget {
			return app.RuleTarget{Rule: domain.RuleJoinMode}
		}, errs.CodeInvalidInput},
		{"expiry without a value", ruleWorldOpts{}, 1, func(*ruleWorld) app.RuleTarget {
			return app.RuleTarget{Rule: domain.RuleExpiry}
		}, errs.CodeInvalidInput},
		{"unknown rule", ruleWorldOpts{}, 1, func(*ruleWorld) app.RuleTarget {
			return app.RuleTarget{Rule: "nope"}
		}, errs.CodeInvalidInput},
		{"max_trade_bps off the step", ruleWorldOpts{}, 1, func(*ruleWorld) app.RuleTarget {
			return intTo(domain.RuleMaxTradeBps, 150)
		}, errs.CodeInvalidInput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := newRuleWorld(t, tc.opts)
			_, err := w.propose(tc.by, tc.target(w))
			if errs.CodeOf(err) != tc.want {
				t.Fatalf("propose = %v, want %s", err, tc.want)
			}
			var n int
			if err := w.pool.QueryRow(t.Context(),
				`SELECT count(*) FROM proposals WHERE cabal_id = $1`, w.cabal.UUID()).Scan(&n); err != nil || n != 0 {
				t.Fatalf("proposals stored = %d, %v, want none", n, err)
			}
		})
	}
}

func TestProposeRuleChange_aNonMemberAndAnOverlongThesisAreRefused(t *testing.T) {
	t.Parallel()
	w := newRuleWorld(t, ruleWorldOpts{})
	_, err := w.handler().Handle(t0(), app.ProposeRuleChange{
		CabalID: w.cabal, ProposerID: ids.UserIDFrom(w.ids.NewV7()), Target: yesTo(true),
	})
	if errs.CodeOf(err) != errs.CodeNotCabalMember {
		t.Fatalf("outsider = %v, want not_cabal_member", err)
	}
	_, err = w.handler().Handle(t0(), app.ProposeRuleChange{
		CabalID: w.cabal, ProposerID: w.members[1], Target: yesTo(true), Thesis: strings.Repeat("x", 281),
	})
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("long thesis = %v, want invalid_input", err)
	}
}

func TestProposeRuleChange_portFailuresSurface(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "boom")
	adminTarget := func(w *ruleWorld) app.RuleTarget { return w.member(domain.RuleAdmin, true, 2) }
	cases := []struct {
		name   string
		op     string
		skip   int
		target func(w *ruleWorld) app.RuleTarget
	}{
		{"IsMember", "IsMember", 0, nil},
		{"Status", "Status", 0, nil},
		{"Rules", "Rules", 0, nil},
		{"VoterSet", "VoterSet", 0, nil},
		{
			"target IsMember",
			"IsMember",
			1,
			func(w *ruleWorld) app.RuleTarget { return w.member(domain.RuleVoter, false, 2) },
		},
		{"target Admins", "Admins", 0, adminTarget},
		{"Cabal", "Cabal", 0, nil},
		{"guard Admins", "Admins", 1, adminTarget},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := newRuleWorld(t, ruleWorldOpts{})
			for range tc.skip {
				w.fake.FailOnce(tc.op, nil)
			}
			w.fake.FailOnce(tc.op, boom)
			target := yesTo(true)
			if tc.target != nil {
				target = tc.target(w)
			}
			if _, err := w.propose(1, target); errs.CodeOf(err) != errs.CodeInternal || !errors.Is(err, boom) {
				t.Fatalf("propose = %v, want the %s failure", err, tc.op)
			}
		})
	}
}
