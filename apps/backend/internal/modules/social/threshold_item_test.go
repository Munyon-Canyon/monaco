package social_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type thresholdRow struct {
	ID, RefID, ActorID, CabalID uuid.UUID
	RefType, Title, Symbol      string
	Status                      *string
	Payload                     feed.Payload
}

func (r proposalRig) thresholds(t *testing.T) []thresholdRow {
	t.Helper()
	rows, err := r.pool.Query(t.Context(), `SELECT id, ref_id, actor_id, cabal_id, ref_type, title,
		coalesce(symbol, ''), status, payload FROM feed_objects WHERE kind = 'threshold' ORDER BY created_at, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []thresholdRow
	for rows.Next() {
		var (
			row thresholdRow
			raw []byte
		)
		err := rows.Scan(&row.ID, &row.RefID, &row.ActorID, &row.CabalID, &row.RefType, &row.Title, &row.Symbol,
			&row.Status, &raw)
		if err != nil {
			t.Fatal(err)
		}
		if row.Payload, err = feed.ParsePayload(raw); err != nil {
			t.Fatal(err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestFeedConsumer_aPassedProposalMakesOneThresholdItem(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	if err := r.create(t); err != nil {
		t.Fatal(err)
	}
	if err := r.passed(t); err != nil {
		t.Fatal(err)
	}
	got := r.thresholds(t)
	if len(got) != 1 {
		t.Fatalf("%d threshold items, want 1", len(got))
	}
	item := got[0]
	if item.RefID != r.proposal || item.RefType != "proposals" || item.ActorID != r.bob.UUID() ||
		item.CabalID != r.cabal || item.Symbol != "AAPLx" || item.Status != nil {
		t.Fatalf("threshold item = %+v, want proposal %s by %s in %s", item, r.proposal, r.bob, r.cabal)
	}
	r.wantThresholdText(t, item)
}

func (r proposalRig) wantThresholdText(t *testing.T, item thresholdRow) {
	t.Helper()
	if want := "bob's proposal to buy $500.00 of Apple was approved"; item.Title != want {
		t.Errorf("title = %q, want %q", item.Title, want)
	}
	want := feed.Payload{
		CabalName: "Alpha", ActorName: "bob", Symbol: "AAPLx", AssetName: "Apple",
		Action: feed.ActionBuy, USDCMicros: money.MicrosFromUint64(500_000_000), ShareOfPotBps: 1000,
		ProposalID: r.proposal,
	}
	if item.Payload != want {
		t.Errorf("payload = %+v, want %+v", item.Payload, want)
	}
	if detail := feed.RenderDetail(feed.KindThreshold, item.Payload); detail != "Execution pending" {
		t.Errorf("detail = %q, want Execution pending", detail)
	}
}

func TestFeedConsumer_aRedeliveredOrRepeatedPassMakesNoSecondThresholdItem(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	if err := r.create(t); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := r.passed(t); err != nil {
			t.Fatal(err)
		}
	}
	if got := r.thresholds(t); len(got) != 1 {
		t.Fatalf("%d threshold items after three passes, want 1", len(got))
	}
}

func TestFeedConsumer_aThresholdItemStaysPendingAfterTheTradeExecutes(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	if err := r.create(t); err != nil {
		t.Fatal(err)
	}
	if err := r.passed(t); err != nil {
		t.Fatal(err)
	}
	before := r.thresholds(t)
	if err := r.executed(t); err != nil {
		t.Fatal(err)
	}
	after := r.thresholds(t)
	if len(after) != 1 || after[0] != before[0] {
		t.Fatalf("threshold items = %+v after execution, want %+v unchanged", after, before)
	}
	if got := r.row(t).Status; got != "executed" {
		t.Fatalf("proposal status = %q, want executed", got)
	}
}

func TestFeedConsumer_aPassBeforeTheProposalItemRetriesAndMakesNoThresholdItem(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	wantCode(t, r.passed(t), errs.CodeFeedItemPending)
	if got := r.thresholds(t); len(got) != 0 {
		t.Fatalf("%d threshold items, want 0", len(got))
	}
	if err := r.create(t); err != nil {
		t.Fatal(err)
	}
	if err := r.passed(t); err != nil {
		t.Fatal(err)
	}
	if got := r.thresholds(t); len(got) != 1 {
		t.Fatalf("%d threshold items after the retry, want 1", len(got))
	}
}

func TestFeedConsumer_otherOutcomesMakeNoThresholdItem(t *testing.T) {
	t.Parallel()
	r := newProposalRig(t)
	if err := r.create(t); err != nil {
		t.Fatal(err)
	}
	if err := r.expired(t); err != nil {
		t.Fatal(err)
	}
	if got := r.thresholds(t); len(got) != 0 {
		t.Fatalf("%d threshold items, want 0", len(got))
	}
}

func TestFeedConsumer_aPassReturnsEachThresholdFailure(t *testing.T) {
	t.Parallel()
	for name, ddl := range map[string]string{
		"source lookup": "ALTER TABLE feed_objects RENAME COLUMN body TO thesis",
		"payload":       `UPDATE feed_objects SET payload = '{"action": 5}' WHERE kind = 'proposal'`,
		"insert":        "ALTER TABLE feed_objects ADD CONSTRAINT no_threshold CHECK (kind <> 'threshold') NOT VALID",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newProposalRig(t)
			if err := r.create(t); err != nil {
				t.Fatal(err)
			}
			if _, err := r.pool.Exec(t.Context(), ddl); err != nil {
				t.Fatal(err)
			}
			if err := r.passed(t); err == nil {
				t.Fatal("ProposalPassed = nil, want an error")
			}
		})
	}
}
