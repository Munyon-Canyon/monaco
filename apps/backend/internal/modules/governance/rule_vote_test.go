package governance_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func (w *ruleWorld) voteHandler() *app.CastVoteHandler {
	return w.handlerFor(w.fake)
}

func (w *ruleWorld) eventTypes(t *testing.T, proposal uuid.UUID) map[string]int {
	t.Helper()
	rows, err := w.pool.Query(
		t.Context(),
		`SELECT type, count(*) FROM events WHERE aggregate_id = $1 GROUP BY type`,
		proposal,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var typ string
		var n int
		if err := rows.Scan(&typ, &n); err != nil {
			t.Fatal(err)
		}
		out[typ] = n
	}
	return out
}

func TestRuleVote_theFifthOfEightYesBallotsPassesItOnItsOwnEvent(t *testing.T) {
	t.Parallel()
	w := newRuleWorld(t, ruleWorldOpts{})
	id := w.mustPropose(t, yesTo(true))
	h := w.voteHandler()
	for i := range 4 {
		got := w.mustCast(t, h, id.UUID(), w.members[i].UUID(), domain.ChoiceYes)
		if got.Status != domain.StatusOpen {
			t.Fatalf("status after %d yes = %s, want open", i+1, got.Status)
		}
	}
	if got := w.mustCast(t, h, id.UUID(), w.members[4].UUID(), domain.ChoiceYes); got.Status != domain.StatusPassed {
		t.Fatalf("status after 5 yes = %s, want passed", got.Status)
	}
	types := w.eventTypes(t, id.UUID())
	if types["proposal.rule_passed"] != 1 || types["proposal.passed"] != 0 || types["vote.cast"] != 5 {
		t.Fatalf("events = %v, want one proposal.rule_passed and no proposal.passed", types)
	}
	w.wantRulePassed(t, id)
}

func (w *ruleWorld) wantRulePassed(t *testing.T, id ids.ProposalID) {
	t.Helper()
	var passed events.ProposalRulePassed
	if err := json.Unmarshal(w.payloads(t, id.UUID(), events.TypeProposalRulePassed)[0], &passed); err != nil {
		t.Fatal(err)
	}
	if passed.ProposerID != w.members[1].UUID() || passed.CabalID != w.cabal.UUID() ||
		!passed.PassedAt.Equal(w.clk.Now()) {
		t.Fatalf("proposal.rule_passed = %+v", passed)
	}
	var change domain.RuleChange
	if err := json.Unmarshal(
		passed.Change,
		&change,
	); err != nil || change.Rule() != domain.RuleShowSides ||
		change.To() != true {
		t.Fatalf("change = %s, %v", passed.Change, err)
	}
}

func TestRuleVote_aFailedRuleProposalEmitsProposalFailedWithTheRuleKind(t *testing.T) {
	t.Parallel()
	w := newRuleWorld(t, ruleWorldOpts{size: 2})
	id := w.mustPropose(t, yesTo(true))
	w.mustCast(t, w.voteHandler(), id.UUID(), w.members[0].UUID(), domain.ChoiceNo)
	types := w.eventTypes(t, id.UUID())
	if types["proposal.failed"] != 1 || types["proposal.rule_passed"] != 0 || types["proposal.passed"] != 0 {
		t.Fatalf("events = %v, want only proposal.failed", types)
	}
	var failed events.ProposalFailed
	if err := json.Unmarshal(
		w.payloads(t, id.UUID(), events.TypeProposalFailed)[0],
		&failed,
	); err != nil ||
		failed.Kind != "rule" {
		t.Fatalf("proposal.failed = %+v, %v, want kind rule", failed, err)
	}
}

func TestRuleVote_anExpiredRuleProposalEmitsProposalExpiredWithTheRuleKind(t *testing.T) {
	t.Parallel()
	w := newRuleWorld(t, ruleWorldOpts{})
	id := w.mustPropose(t, yesTo(true))
	w.clk.Advance(48 * time.Hour)
	report, err := w.expiry().Tick(observability.WithActor(t.Context(), "system:poller.governance.proposal_expiry"))
	if err != nil || report.Changed < 1 {
		t.Fatalf("Tick = %+v, %v, want the proposal expired", report, err)
	}
	var expired events.ProposalExpired
	raws := w.payloads(t, id.UUID(), events.TypeProposalExpired)
	if len(raws) != 1 || json.Unmarshal(raws[0], &expired) != nil || expired.Kind != "rule" {
		t.Fatalf("proposal.expired = %s, want one with kind rule", raws)
	}
}
