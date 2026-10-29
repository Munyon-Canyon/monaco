package cabal_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func passedAt() time.Time { return time.Date(2026, 10, 20, 9, 41, 0, 0, time.UTC) }

func rulePassed(c testkit.SeededCabal, proposer ids.UserID, change string) events.ProposalRulePassed {
	return events.ProposalRulePassed{
		V: 1, ProposalID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(change)), CabalID: c.ID.UUID(),
		ProposerID: proposer.UUID(), Change: json.RawMessage(change), PassedAt: passedAt(),
	}
}

func (f accessFixture) deliver(ctx context.Context, e events.ProposalRulePassed) error {
	ctx = observability.WithActor(ctx, "system:consumer.cabal.rule_passed")
	return f.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return adapters.RulePassed{}.Handle(ctx, tx, e, f.clock.Now())
	})
}

func boolChange(rule string, to bool, user *uuid.UUID) string {
	who := ""
	if user != nil {
		who = fmt.Sprintf(`,"user_id":%q`, user.String())
	}
	return fmt.Sprintf(`{"v":1,"rule":%q,"from":%t,"to":%t%s}`, rule, !to, to, who)
}

func valueChange(rule, to string) string {
	return fmt.Sprintf(`{"v":1,"rule":%q,"from":null,"to":%s}`, rule, to)
}

type storedRules struct {
	showSides               bool
	showSidesAt             *time.Time
	maxTradeBps             *int32
	threshold, join, voters string
	expiry                  int32
}

func (f accessFixture) rules(t *testing.T, c testkit.SeededCabal) storedRules {
	t.Helper()
	var r storedRules
	if err := f.pool.QueryRow(t.Context(), `SELECT show_sides, show_sides_changed_at, max_trade_bps, threshold,
		join_mode, voter_mode, proposal_expiry_seconds FROM cabals WHERE id = $1`, c.ID.UUID()).Scan(
		&r.showSides, &r.showSidesAt, &r.maxTradeBps, &r.threshold, &r.join, &r.voters, &r.expiry); err != nil {
		t.Fatal(err)
	}
	if r.showSidesAt != nil {
		at := r.showSidesAt.UTC()
		r.showSidesAt = &at
	}
	return r
}

func (f accessFixture) role(t *testing.T, c testkit.SeededCabal, user ids.UserID) (string, bool) {
	t.Helper()
	var role string
	var vote bool
	if err := f.pool.QueryRow(t.Context(),
		`SELECT role, can_vote FROM cabal_members WHERE cabal_id = $1 AND user_id = $2`,
		c.ID.UUID(), user.UUID()).Scan(&role, &vote); err != nil {
		t.Fatal(err)
	}
	return role, vote
}

func TestApplyRuleChange_showSidesStampsThePassedTimeAndNamesTheProposer(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	e := rulePassed(c, c.Members[1].ID, boolChange("show_sides", true, nil))
	for range 2 {
		if err := f.deliver(t.Context(), e); err != nil {
			t.Fatal(err)
		}
	}
	got := f.rules(t, c)
	if !got.showSides || got.showSidesAt == nil || !got.showSidesAt.Equal(passedAt()) {
		t.Fatalf("rules = %+v, want show_sides on since %s", got, passedAt())
	}
	updates := f.updates(t)
	on := true
	if len(updates) != 1 || updates[0].ActorID != c.Members[1].ID.UUID() || updates[0].Changes.ShowSides == nil ||
		*updates[0].Changes.ShowSides != on || *updates[0].Changes.ProposalID != e.ProposalID {
		t.Fatalf("cabal.updated = %+v, want one by the proposer naming the proposal", updates)
	}
}

func TestApplyRuleChange_maxTradeBpsSetsThenClears(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	if err := f.deliver(t.Context(), rulePassed(c, c.Members[1].ID, valueChange("max_trade_bps", "2000"))); err != nil {
		t.Fatal(err)
	}
	if got := f.rules(t, c); got.maxTradeBps == nil || *got.maxTradeBps != 2000 {
		t.Fatalf("max_trade_bps = %v, want 2000", got.maxTradeBps)
	}
	if err := f.deliver(t.Context(), rulePassed(c, c.Members[1].ID, valueChange("max_trade_bps", "null"))); err != nil {
		t.Fatal(err)
	}
	updates := f.updates(t)
	if got := f.rules(t, c); got.maxTradeBps != nil || len(updates) != 2 || !updates[1].Changes.MaxTradeBpsCleared {
		t.Fatalf("max_trade_bps = %v, updates %+v; want it cleared and announced", got.maxTradeBps, updates)
	}
}

func TestApplyRuleChange_appliesThresholdExpiryAndJoinMode(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	for _, change := range []string{
		valueChange("threshold", `"unanimous"`), valueChange("proposal_expiry_seconds", "3600"),
		valueChange("join_mode", `"request"`),
	} {
		if err := f.deliver(t.Context(), rulePassed(c, c.Members[1].ID, change)); err != nil {
			t.Fatal(err)
		}
	}
	got := f.rules(t, c)
	if got.threshold != "unanimous" || got.expiry != 3600 || got.join != "request" || len(f.updates(t)) != 3 {
		t.Fatalf("rules = %+v with %d events, want all three applied", got, len(f.updates(t)))
	}
}

func TestApplyRuleChange_voterRemovalSwitchesAllToListAndAddKeepsTheList(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	second, third := c.Members[1].ID.UUID(), c.Members[2].ID.UUID()
	if err := f.deliver(t.Context(), rulePassed(c, c.Creator.ID, boolChange("voter", false, &third))); err != nil {
		t.Fatal(err)
	}
	if got := f.voters(t, c); !slices.Equal(got, []bool{true, true, false}) || f.rules(t, c).voters != "list" {
		t.Fatalf("voters = %v mode %s, want the third removed in list mode", got, f.rules(t, c).voters)
	}
	if err := f.deliver(t.Context(), rulePassed(c, c.Creator.ID, boolChange("voter", false, &second))); err != nil {
		t.Fatal(err)
	}
	if err := f.deliver(t.Context(), rulePassed(c, c.Creator.ID, boolChange("voter", true, &third))); err != nil {
		t.Fatal(err)
	}
	if got := f.voters(t, c); !slices.Equal(got, []bool{true, false, true}) {
		t.Fatalf("voters = %v, want the creator and the third", got)
	}
}

func TestApplyRuleChange_adminAddSetsCanVoteAndRemoveKeepsTheMember(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3), testkit.WithVoterMode("list"))
	user := c.Members[1].ID.UUID()
	if err := f.deliver(t.Context(), rulePassed(c, c.Creator.ID, boolChange("admin", true, &user))); err != nil {
		t.Fatal(err)
	}
	if role, vote := f.role(t, c, c.Members[1].ID); role != "admin" || !vote {
		t.Fatalf("after add role = %s vote = %v, want an admin who votes", role, vote)
	}
	if err := f.deliver(t.Context(), rulePassed(c, c.Creator.ID, boolChange("admin", false, &user))); err != nil {
		t.Fatal(err)
	}
	updates := f.updates(t)
	if role, _ := f.role(t, c, c.Members[1].ID); role != "member" || len(updates) != 2 ||
		len(updates[0].Changes.AdminIDs) != 2 || len(updates[1].Changes.AdminIDs) != 1 {
		t.Fatalf("after remove role = %s updates %+v, want a member and both admin lists announced", role, updates)
	}
}

func TestApplyRuleChange_acksWithoutAWriteWhenTheChangeIsNoLongerValid(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	admin, creator := c.Members[1].ID.UUID(), c.Creator.ID.UUID()
	gone := uuid.NewSHA1(uuid.NameSpaceOID, []byte("gone"))
	f.makeAdmin(t, c.ID, c.Members[1].ID)
	for name, change := range map[string]string{
		"voter removal of an admin":   boolChange("voter", false, &admin),
		"voter change of the creator": boolChange("voter", false, &creator),
		"admin change of the creator": boolChange("admin", false, &creator),
		"a member who left":           boolChange("voter", false, &gone),
		"a voter already voting":      boolChange("voter", true, &admin),
		"an admin already admin":      boolChange("admin", true, &admin),
		"sides already off":           boolChange("show_sides", false, nil),
		"a bad cap":                   valueChange("max_trade_bps", "150"),
		"no cap already":              valueChange("max_trade_bps", "null"),
		"an unknown threshold":        valueChange("threshold", `"most"`),
		"an unchanged expiry":         valueChange("proposal_expiry_seconds", "86400"),
	} {
		if err := f.deliver(t.Context(), rulePassed(c, c.Members[2].ID, change)); err != nil {
			t.Errorf("%s: %v, want an ack", name, err)
		}
	}
	if len(f.updates(t)) != 0 {
		t.Fatalf("events = %+v, want none", f.updates(t))
	}
	f.exec(t, `UPDATE cabals SET status = 'banned' WHERE id = $1`, c.ID.UUID())
	if err := f.deliver(t.Context(), rulePassed(c, c.Members[2].ID, boolChange("show_sides", true, nil))); err != nil {
		t.Fatal(err)
	}
	unknown := testkit.SeededCabal{ID: ids.CabalIDFrom(uuid.NewSHA1(uuid.NameSpaceOID, []byte("no cabal")))}
	if err := f.deliver(
		t.Context(),
		rulePassed(unknown, c.Members[2].ID, boolChange("show_sides", true, nil)),
	); err != nil {
		t.Fatal(err)
	}
	if got := f.rules(t, c); got.showSides || len(f.updates(t)) != 0 {
		t.Fatalf("rules = %+v, events %+v; want no write for a banned or missing cabal", got, f.updates(t))
	}
}

func TestApplyRuleChange_rejectsAnUndecodableChange(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	for _, change := range []string{
		`not json`, `{"v":2,"rule":"show_sides","to":true}`, `{"v":1,"rule":"colour","to":true}`,
		`{"v":1,"rule":"show_sides","to":"yes"}`, `{"v":1,"rule":"voter","to":true}`,
	} {
		if err := f.deliver(
			t.Context(),
			rulePassed(c, c.Creator.ID, change),
		); errs.CodeOf(
			err,
		) != errs.CodeDecodeFailed &&
			change != `{"v":1,"rule":"voter","to":true}` {
			t.Errorf("%s: err = %v, want decode_failed", change, err)
		}
	}
}

func TestApplyRuleChange_wrapsStoreFailuresAsInternal(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, voterMode, statement, change string
	}{
		{"the cabal lock", "all", `ALTER TABLE cabals RENAME TO cabals_gone`, boolChange("show_sides", true, nil)},
		{"the member lock", "all", `ALTER TABLE cabal_members RENAME TO members_gone`, boolChange("show_sides", true, nil)},
		{"the cabal read", "all", `ALTER TABLE cabals DROP COLUMN max_trade_bps`, boolChange("show_sides", true, nil)},
		{"the member list", "all", `ALTER TABLE cabal_members DROP COLUMN joined_at`, boolChange("show_sides", true, nil)},
		{
			"the rules write", "all", `ALTER TABLE cabals ADD CONSTRAINT sides_off CHECK (NOT show_sides)`,
			boolChange("show_sides", true, nil),
		},
		{
			"the voter write", "list",
			`ALTER TABLE cabal_members ADD CONSTRAINT creator_votes CHECK (NOT can_vote OR role = 'creator')`, "voter",
		},
		{
			"the role write", "list", `ALTER TABLE cabal_members ADD CONSTRAINT no_admin CHECK (role <> 'admin')`, "admin",
		},
		{
			"the event append", "all", `ALTER TABLE events RENAME TO events_gone`, boolChange("show_sides", true, nil),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newAccess(t)
			c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2), testkit.WithVoterMode(tt.voterMode))
			user := c.Members[1].ID.UUID()
			change := tt.change
			if change == "voter" || change == "admin" {
				change = boolChange(change, true, &user)
			}
			f.exec(t, tt.statement)
			wantErr(t, f.deliver(t.Context(), rulePassed(c, c.Creator.ID, change)), errs.CodeInternal)
		})
	}
}
