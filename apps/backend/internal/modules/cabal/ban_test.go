package cabal_test

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f createFixture) ban(ctx context.Context, e events.AdminCabalBanApproved) error {
	h := adapters.Ban{IDs: f.ids}
	return f.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error { return h.Handle(ctx, tx, e, f.clock.Now()) })
}

func (f createFixture) approved(cabal testkit.SeededCabal, reason string) events.AdminCabalBanApproved {
	return events.AdminCabalBanApproved{
		V: 1, ApprovalID: f.ids.NewV7(), CabalID: cabal.ID.UUID(), RequestedBy: f.ids.NewV7(),
		ApprovedBy: f.ids.NewV7(), Reason: reason,
	}
}

func (f createFixture) status(t *testing.T, cabal testkit.SeededCabal) (status string) {
	t.Helper()
	if err := f.pool.QueryRow(t.Context(), `SELECT status FROM cabals WHERE id = $1`, cabal.ID.UUID()).
		Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func (f createFixture) payloads(t *testing.T, typ events.Type) []map[string]any {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT payload FROM events WHERE type = $1 ORDER BY id`, string(typ))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var raw []byte
		var payload map[string]any
		if err := rows.Scan(&raw); err != nil || json.Unmarshal(raw, &payload) != nil {
			t.Fatalf("scan %s: %v", typ, err)
		}
		out = append(out, payload)
	}
	return out
}

func system(t *testing.T) context.Context {
	t.Helper()
	return observability.WithActor(t.Context(), "system:consumer.cabal.ban")
}

func TestBan_BansAnActiveCabalAndRecordsTheActionOnce(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	cabal := testkit.NewCabal(t, f.pool)
	e := f.approved(cabal, "scam cabal")
	for range 2 {
		if err := f.ban(system(t), e); err != nil {
			t.Fatal(err)
		}
	}
	if got := f.status(t, cabal); got != "banned" {
		t.Fatalf("status = %s, want banned", got)
	}
	banned := f.payloads(t, events.TypeCabalBanned)
	if len(banned) != 1 || banned[0]["cabal_id"] != cabal.ID.String() || banned[0]["reason"] != "scam cabal" {
		t.Fatalf("cabal.banned = %v, want one for the cabal", banned)
	}
	actions := f.payloads(t, events.TypeAdminAction)
	if len(actions) != 1 {
		t.Fatalf("admin.action = %v, want one", actions)
	}
	assertBanAction(t, actions[0], cabal, e)
}

func assertBanAction(t *testing.T, a map[string]any, cabal testkit.SeededCabal, e events.AdminCabalBanApproved) {
	t.Helper()
	before, _ := json.Marshal(a["before"])
	after, _ := json.Marshal(a["after"])
	got := []any{
		a["action"], a["target_type"], a["target_id"], a["admin_id"], a["approved_by"], a["reason"],
		string(before), string(after),
	}
	want := []any{
		"cabal_ban", "cabal", cabal.ID.String(), e.RequestedBy.String(), e.ApprovedBy.String(), "scam cabal",
		`{"status":"active"}`, `{"status":"banned"}`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("admin.action = %v, want %v", got, want)
	}
}

func TestBan_LeavesAnUnknownOrBannedCabalAlone(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	cabal := testkit.NewCabal(t, f.pool)
	unknown := events.AdminCabalBanApproved{V: 1, CabalID: f.ids.NewV7(), Reason: "scam cabal"}
	if err := f.ban(system(t), unknown); err != nil {
		t.Fatalf("ban of an unknown cabal = %v, want nil", err)
	}
	if n := len(f.payloads(t, events.TypeCabalBanned)); n != 0 {
		t.Fatalf("cabal.banned count = %d, want none", n)
	}
	if err := f.ban(system(t), f.approved(cabal, "scam cabal")); err != nil {
		t.Fatal(err)
	}
	if err := f.ban(system(t), f.approved(cabal, "second try")); err != nil {
		t.Fatal(err)
	}
	if n := len(f.payloads(t, events.TypeCabalBanned)); n != 1 {
		t.Fatalf("cabal.banned count = %d, want the first ban only", n)
	}
}

func TestBan_RollsBackWhenItCannotRecordTheBan(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	cabal := testkit.NewCabal(t, f.pool)
	if err := f.ban(system(t), f.approved(cabal, "x")); errs.CodeOf(err) != errs.CodeReasonRequired {
		t.Fatalf("ban with a short reason = %v, want reason_required", err)
	}
	if err := f.ban(t.Context(), f.approved(cabal, "scam cabal")); err == nil {
		t.Fatal("ban without an actor = nil error, want the event append to fail")
	}
	if got := f.status(t, cabal); got != "active" {
		t.Fatalf("status = %s, want the failed bans rolled back", got)
	}
}

func TestBan_FailsWhenTheStoreIsGone(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	cabal := testkit.NewCabal(t, f.pool)
	if _, err := f.pool.Exec(t.Context(), `ALTER TABLE cabals RENAME TO cabals_gone`); err != nil {
		t.Fatal(err)
	}
	if err := f.ban(system(t), f.approved(cabal, "scam cabal")); err == nil {
		t.Fatal("ban with the cabals table gone = nil error")
	}
}
