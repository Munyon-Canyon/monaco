package funding_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

const (
	adminReason = `{"reason":"investigating"}`
	adminPause  = "/v1/admin/pause"
	adminResume = "/v1/admin/resume"
)

func cabalAdminPath(cabal ids.CabalID, verb string) string {
	return "/v1/admin/cabals/" + cabal.String() + "/" + verb
}

func asOperator() []scenario.Step {
	return []scenario.Step{scenario.SeededAdmin("admin", "operator"), scenario.AsUser("admin")}
}

func expectPauseState(reasons ...string) scenario.Step {
	return scenario.ExpectField("reasons", func(s *scenario.Scenario, raw json.RawMessage) {
		var got []string
		if err := json.Unmarshal(raw, &got); err != nil || !slices.Equal(got, slices.Clone(reasons)) {
			s.Fatalf("reasons = %s (%v), want %v", raw, err, reasons)
		}
	})
}

func isPaused(t *testing.T, pool *pgxpool.Pool, cabal ids.CabalID) bool {
	t.Helper()
	pause, err := adapters.NewPauses(pool).IsPaused(t.Context(), cabal)
	if err != nil {
		t.Fatal(err)
	}
	return pause.Paused
}

func adminActionPayload(t *testing.T, pool *pgxpool.Pool, kind events.AdminActionKind) events.AdminAction {
	t.Helper()
	var raw []byte
	err := pool.QueryRow(t.Context(),
		`SELECT payload FROM events WHERE type = $1 AND payload->>'action' = $2`, events.TypeAdminAction, string(kind),
	).Scan(&raw)
	if err != nil {
		t.Fatal(err)
	}
	var action events.AdminAction
	if err := json.Unmarshal(raw, &action); err != nil {
		t.Fatal(err)
	}
	return action
}

func pausedReasons(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var state struct {
		PausedReasons []string `json:"paused_reasons"`
	}
	if err := json.Unmarshal(raw, &state); err != nil || state.PausedReasons == nil {
		t.Fatalf("paused_reasons in %s: %v, want a list", raw, err)
	}
	return state.PausedReasons
}

func TestAdminPause_Cabal_Ok(t *testing.T) {
	t.Parallel()
	s := flow15Scenario(t)
	cabal := testkit.NewCabal(t, s.DB())
	s.Given(asOperator()...).
		When(
			scenario.Post(cabalAdminPath(cabal.ID, "pause"), adminReason),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("paused", true),
			expectPauseState("ops"),
		).
		Then(
			scenario.ExpectAdminAction(events.AdminActionOpsPause, cabal.ID.String()),
			scenario.ExpectEvents(events.TypeCabalPaused, 1),
		)
	if !isPaused(t, s.DB(), cabal.ID) {
		t.Fatal("IsPaused = false after the admin pause")
	}
	var note string
	var createdBy ids.UserID
	if err := s.DB().QueryRow(t.Context(),
		`SELECT note, created_by FROM cabal_pauses WHERE cabal_id = $1 AND reason = 'ops'`, cabal.ID.UUID(),
	).Scan(&note, &createdBy); err != nil {
		t.Fatal(err)
	}
	action := adminActionPayload(t, s.DB(), events.AdminActionOpsPause)
	if note != "investigating" || createdBy.UUID() != action.AdminID || action.Reason != "investigating" ||
		action.TargetType != events.AdminTargetCabal ||
		len(pausedReasons(t, action.Before)) != 0 || !slices.Equal(pausedReasons(t, action.After), []string{"ops"}) {
		t.Fatalf("note=%q created_by=%s action=%+v, want the reason on both and the reasons before and after",
			note, createdBy, action)
	}
}

func TestAdminPause_Global_BlocksEveryCabal(t *testing.T) {
	t.Parallel()
	s := flow15Scenario(t)
	first, second := testkit.NewCabal(t, s.DB()), testkit.NewCabal(t, s.DB())
	s.Given(asOperator()...).
		When(
			scenario.Post(adminPause, adminReason),
			scenario.ExpectStatus(http.StatusOK),
			expectPauseState("ops"),
		).
		Then(
			scenario.ExpectAdminAction(events.AdminActionGlobalPause, "global"),
			scenario.ExpectEvents(events.TypeCabalPaused, 1),
		)
	if !isPaused(t, s.DB(), first.ID) || !isPaused(t, s.DB(), second.ID) {
		t.Fatal("IsPaused = false for a cabal after the global pause")
	}
	if action := adminActionPayload(
		t,
		s.DB(),
		events.AdminActionGlobalPause,
	); action.TargetType != events.AdminTargetGlobal {
		t.Fatalf("target_type = %s, want global", action.TargetType)
	}
}

func TestAdminResume_Cabal_Ok(t *testing.T) {
	t.Parallel()
	s := flow15Scenario(t)
	cabal := testkit.NewCabal(t, s.DB())
	s.Given(asOperator()...).
		When(
			scenario.Post(cabalAdminPath(cabal.ID, "pause"), adminReason),
			scenario.ExpectStatus(http.StatusOK),
			scenario.Post(cabalAdminPath(cabal.ID, "resume"), `{"reason":"resolved"}`),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("paused", false),
			scenario.ExpectJSON("since", nil),
			expectPauseState(),
		).
		Then(
			scenario.ExpectAdminAction(events.AdminActionOpsResume, cabal.ID.String()),
			scenario.ExpectEvents(events.TypeCabalResumed, 1),
		)
	if isPaused(t, s.DB(), cabal.ID) {
		t.Fatal("IsPaused = true after the admin resume")
	}
	action := adminActionPayload(t, s.DB(), events.AdminActionOpsResume)
	if !slices.Equal(pausedReasons(t, action.Before), []string{"ops"}) || len(pausedReasons(t, action.After)) != 0 {
		t.Fatalf("before=%s after=%s, want ops then none", action.Before, action.After)
	}
}

func TestAdminResume_Global_LeavesACabalPauseAlone(t *testing.T) {
	t.Parallel()
	s := flow15Scenario(t)
	cabal := testkit.NewCabal(t, s.DB())
	s.Given(asOperator()...).
		When(
			scenario.Post(adminPause, adminReason),
			scenario.ExpectStatus(http.StatusOK),
			scenario.Post(cabalAdminPath(cabal.ID, "pause"), adminReason),
			scenario.ExpectStatus(http.StatusOK),
			scenario.Post(adminResume, `{"reason":"resolved"}`),
			scenario.ExpectStatus(http.StatusOK),
			scenario.ExpectJSON("paused", false),
		).
		Then(
			scenario.ExpectAdminAction(events.AdminActionGlobalResume, "global"),
			scenario.ExpectEvents(events.TypeCabalResumed, 1),
		)
	if !isPaused(t, s.DB(), cabal.ID) {
		t.Fatal("IsPaused = false, want the cabal's own ops pause to survive the global resume")
	}
}

func TestAdminResume_KeepsExternalDepositPause(t *testing.T) {
	t.Parallel()
	s := flow15Scenario(t)
	cabal := testkit.NewCabal(t, s.DB())
	if _, err := s.DB().Exec(t.Context(), `INSERT INTO cabal_pauses (id, cabal_id, reason, created_at)
		VALUES (gen_random_uuid(), $1, 'external_deposit', now())`, cabal.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	s.Given(asOperator()...).
		When(
			scenario.Post(cabalAdminPath(cabal.ID, "pause"), adminReason),
			scenario.ExpectStatus(http.StatusOK),
			expectPauseState("external_deposit", "ops"),
			scenario.Post(cabalAdminPath(cabal.ID, "resume"), `{"reason":"resolved"}`),
			scenario.ExpectProblem(errs.CodeCabalStillPaused),
			scenario.ExpectStatus(http.StatusConflict),
		).
		Then(
			scenario.ExpectAdminAction(events.AdminActionOpsResume, cabal.ID.String()),
			scenario.ExpectEvents(events.TypeCabalResumed, 0),
		)
	if got := openPauses(t, s.DB()); !slices.Equal(got, []string{"external_deposit"}) {
		t.Fatalf("open pauses = %v, want the ops row resolved and the external-deposit row kept", got)
	}
	action := adminActionPayload(t, s.DB(), events.AdminActionOpsResume)
	if !slices.Equal(pausedReasons(t, action.After), []string{"external_deposit"}) {
		t.Fatalf("after = %s, want the surviving reason", action.After)
	}
}

func TestAdminPause_AlreadyPaused(t *testing.T) {
	t.Parallel()
	s := flow15Scenario(t)
	cabal := testkit.NewCabal(t, s.DB())
	s.Given(asOperator()...).
		When(
			scenario.Post(cabalAdminPath(cabal.ID, "pause"), adminReason),
			scenario.ExpectStatus(http.StatusOK),
			scenario.Post(cabalAdminPath(cabal.ID, "pause"), `{"reason":"again"}`),
			scenario.ExpectStatus(http.StatusConflict),
			scenario.ExpectProblem(errs.CodeAlreadyPaused),
			scenario.Post(adminPause, adminReason),
			scenario.ExpectStatus(http.StatusOK),
			scenario.Post(adminPause, `{"reason":"again"}`),
			scenario.ExpectStatus(http.StatusConflict),
			scenario.ExpectProblem(errs.CodeAlreadyPaused),
		).
		Then(
			scenario.ExpectEvents(events.TypeCabalPaused, 2),
			scenario.ExpectEvents(events.TypeAdminAction, 2),
		)
	if got := openPauses(t, s.DB()); !slices.Equal(got, []string{"ops", "ops"}) {
		t.Fatalf("open pauses = %v, want one ops row per scope", got)
	}
}

func TestAdminResume_NoOpsPause(t *testing.T) {
	t.Parallel()
	s := flow15Scenario(t)
	cabal := testkit.NewCabal(t, s.DB())
	s.Given(asOperator()...).
		When(
			scenario.Post(cabalAdminPath(cabal.ID, "resume"), adminReason),
			scenario.ExpectStatus(http.StatusConflict),
			scenario.ExpectProblem(errs.CodeNoOpsPause),
			scenario.Post(adminResume, adminReason),
			scenario.ExpectStatus(http.StatusConflict),
			scenario.ExpectProblem(errs.CodeNoOpsPause),
		).
		Then(
			scenario.ExpectEvents(events.TypeCabalResumed, 0),
			scenario.ExpectEvents(events.TypeAdminAction, 0),
		)
}

func TestAdminPause_ModeratorForbidden(t *testing.T) {
	t.Parallel()
	s := flow15Scenario(t)
	cabal := testkit.NewCabal(t, s.DB())
	s.Given(scenario.SeededAdmin("admin", "moderator"), scenario.AsUser("admin")).
		When(
			scenario.Post(cabalAdminPath(cabal.ID, "pause"), adminReason),
			scenario.ExpectStatus(http.StatusForbidden),
			scenario.ExpectProblem(errs.CodeAdminForbidden),
			scenario.Post(adminPause, adminReason),
			scenario.ExpectProblem(errs.CodeAdminForbidden),
			scenario.Post(cabalAdminPath(cabal.ID, "resume"), adminReason),
			scenario.ExpectProblem(errs.CodeAdminForbidden),
			scenario.Post(adminResume, adminReason),
			scenario.ExpectProblem(errs.CodeAdminForbidden),
		).
		Then(
			scenario.ExpectEvents(events.TypeCabalPaused, 0),
			scenario.ExpectEvents(events.TypeAdminAction, 0),
		)
}

func TestAdminPause_RefusesAShortReasonAndAnUnknownCabal(t *testing.T) {
	t.Parallel()
	s := flow15Scenario(t)
	cabal := testkit.NewCabal(t, s.DB())
	missing := ids.CabalIDFrom(testkit.NewIDs(50).NewV7())
	s.Given(asOperator()...).
		When(
			scenario.Post(cabalAdminPath(cabal.ID, "pause"), `{"reason":"  "}`),
			scenario.ExpectProblem(errs.CodeReasonRequired),
			scenario.Post(cabalAdminPath(missing, "pause"), adminReason),
			scenario.ExpectStatus(http.StatusNotFound),
			scenario.ExpectProblem(errs.CodeCabalNotFound),
			scenario.Post(cabalAdminPath(missing, "resume"), adminReason),
			scenario.ExpectProblem(errs.CodeCabalNotFound),
		).
		Then(
			scenario.ExpectEvents(events.TypeCabalPaused, 0),
			scenario.ExpectEvents(events.TypeAdminAction, 0),
		)
}
