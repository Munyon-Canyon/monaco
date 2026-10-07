package events_test

import (
	"encoding/json"
	"io/fs"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	actionIDText = "01890a5d-ac96-774b-bcce-b302099a805a"
	adminIDText  = "01890a5d-ac96-774b-bcce-b302099a8059"
	pingIDText   = "01890a5d-ac96-774b-bcce-b302099a8057"
)

func mustUUID(t *testing.T, raw string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustAdmin(t *testing.T) ids.UserID {
	t.Helper()
	admin, err := ids.ParseUserID(adminIDText)
	if err != nil {
		t.Fatal(err)
	}
	return admin
}

func mustReason(t *testing.T, raw string) events.Reason {
	t.Helper()
	reason, err := events.NewReason(raw)
	if err != nil {
		t.Fatal(err)
	}
	return reason
}

func TestNewReason_countsRunesAfterTrimmingAndKeepsTheTrimmedText(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{"three runes", "abc", "abc", true},
		{"three runes of two bytes each", "ééé", "ééé", true},
		{"trims the edges", "  spam  ", "spam", true},
		{"500 runes of two bytes each", strings.Repeat("é", 500), strings.Repeat("é", 500), true},
		{"500 runes inside edge spaces", " " + strings.Repeat("a", 500) + "\n", strings.Repeat("a", 500), true},
		{"empty", "", "", false},
		{"whitespace only", " \t\n ", "", false},
		{"two runes", "ab", "", false},
		{"two runes inside edge spaces", "  ab  ", "", false},
		{"501 runes", strings.Repeat("é", 501), "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := events.NewReason(tc.raw)
			switch {
			case tc.ok && (err != nil || got.String() != tc.want):
				t.Fatalf("NewReason(%d runes) = %q, %v, want %q", len([]rune(tc.raw)), got.String(), err, tc.want)
			case !tc.ok && (errs.CodeOf(err) != errs.CodeReasonRequired || got.String() != ""):
				t.Fatalf("NewReason(%d runes) = %q, %v, want the zero Reason and reason_required",
					len([]rune(tc.raw)), got.String(), err)
			}
		})
	}
}

func TestAdminActionKind_validIsExactlyTheTwelveWireValues(t *testing.T) {
	t.Parallel()
	wire := map[string]events.AdminActionKind{
		"ops_pause":       events.AdminActionOpsPause,
		"ops_resume":      events.AdminActionOpsResume,
		"global_pause":    events.AdminActionGlobalPause,
		"global_resume":   events.AdminActionGlobalResume,
		"proposal_void":   events.AdminActionProposalVoid,
		"user_ban":        events.AdminActionUserBan,
		"user_unban":      events.AdminActionUserUnban,
		"comment_remove":  events.AdminActionCommentRemove,
		"handle_revoke":   events.AdminActionHandleRevoke,
		"handle_reassign": events.AdminActionHandleReassign,
		"cabal_ban":       events.AdminActionCabalBan,
		"ping_flag":       events.AdminActionPingFlag,
	}
	for text, kind := range wire {
		if string(kind) != text || !kind.Valid() {
			t.Errorf("%q: wire value %q, Valid = %t, want the same text and true", text, kind, kind.Valid())
		}
	}
	for _, bad := range []events.AdminActionKind{"", "OPS_PAUSE", "ops-pause", "pause", "ping_flag "} {
		if bad.Valid() {
			t.Errorf("AdminActionKind(%q).Valid = true, want false", bad)
		}
	}
}

func TestAdminTargetType_validIsExactlyTheSevenWireValues(t *testing.T) {
	t.Parallel()
	wire := map[string]events.AdminTargetType{
		"cabal":       events.AdminTargetCabal,
		"proposal":    events.AdminTargetProposal,
		"user":        events.AdminTargetUser,
		"comment":     events.AdminTargetComment,
		"handle":      events.AdminTargetHandle,
		"system_ping": events.AdminTargetSystemPing,
		"global":      events.AdminTargetGlobal,
	}
	for text, target := range wire {
		if string(target) != text || !target.Valid() {
			t.Errorf("%q: wire value %q, Valid = %t, want the same text and true", text, target, target.Valid())
		}
	}
	for _, bad := range []events.AdminTargetType{"", "Cabal", "system-ping", "ping"} {
		if bad.Valid() {
			t.Errorf("AdminTargetType(%q).Valid = true, want false", bad)
		}
	}
}

func TestNewAdminAction_emitsExactlyTheGoldenPayload(t *testing.T) {
	t.Parallel()
	flaggedAt := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	got, err := events.NewAdminAction(
		mustUUID(t, actionIDText), mustAdmin(t), events.AdminActionPingFlag, events.AdminTargetSystemPing,
		pingIDText, mustReason(t, "spam"),
		map[string]any{"flagged_at": nil}, map[string]any{"flagged_at": flaggedAt})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want, err := fs.ReadFile(golden(), goldenName(events.TypeAdminAction, 1))
	if err != nil {
		t.Fatal(err)
	}
	if string(payload)+"\n" != string(want) {
		t.Fatalf("NewAdminAction payload\n%s\nwant the golden\n%s", payload, want)
	}
}

func TestNewAdminAction_aNilStateIsJSONNullAndApprovalIsLeftEmpty(t *testing.T) {
	t.Parallel()
	got, err := events.NewAdminAction(
		mustUUID(t, actionIDText), mustAdmin(t), events.AdminActionGlobalPause, events.AdminTargetGlobal,
		"global", mustReason(t, "market halt"), nil, map[string]any{"paused": true})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Before) != "null" || string(got.After) != `{"paused":true}` || got.ApprovedBy != nil || got.V != 1 {
		t.Fatalf("NewAdminAction = before %s, after %s, approved_by %v, v %d, want null, the paused object, nil and 1",
			got.Before, got.After, got.ApprovedBy, got.V)
	}
}

type auditCall struct {
	reason   events.Reason
	action   events.AdminActionKind
	target   events.AdminTargetType
	targetID string
	before   any
	after    any
}

func TestNewAdminAction_refusesWhatCannotBeAudited(t *testing.T) {
	t.Parallel()
	id, admin, reason := mustUUID(t, actionIDText), mustAdmin(t), mustReason(t, "spam")
	for _, tc := range []struct {
		name   string
		mutate func(*auditCall)
		want   errs.Code
	}{
		{"the zero reason", func(c *auditCall) { c.reason = events.Reason{} }, errs.CodeReasonRequired},
		{"an unknown action", func(c *auditCall) { c.action = "ping-flag" }, errs.CodeInternal},
		{"an unknown target type", func(c *auditCall) { c.target = "ping" }, errs.CodeInternal},
		{"an empty target id", func(c *auditCall) { c.targetID = "" }, errs.CodeInternal},
		{"a before state that is not JSON", func(c *auditCall) { c.before = math.NaN() }, errs.CodeInternal},
		{"an after state that is not JSON", func(c *auditCall) { c.after = make(chan int) }, errs.CodeInternal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := auditCall{
				reason: reason, action: events.AdminActionPingFlag, target: events.AdminTargetSystemPing,
				targetID: pingIDText,
			}
			tc.mutate(&c)
			got, err := events.NewAdminAction(id, admin, c.action, c.target, c.targetID, c.reason, c.before, c.after)
			if errs.CodeOf(err) != tc.want || !reflect.DeepEqual(got, events.AdminAction{}) {
				t.Fatalf("NewAdminAction = %+v, %v, want the zero event and %s", got, err, tc.want)
			}
		})
	}
}

func TestAdminActionAggregatesOnItsOwnID(t *testing.T) {
	t.Parallel()
	id := mustUUID(t, actionIDText)
	var ev events.Event = events.AdminAction{V: 1, ActionID: id}
	if ev.Type() != events.TypeAdminAction || ev.AggregateType() != "admin_action" || ev.AggregateID() != id {
		t.Fatalf("AdminAction aggregate = %s %s %s", ev.Type(), ev.AggregateType(), ev.AggregateID())
	}
}

func TestSystemPingFlaggedAggregatesOnThePing(t *testing.T) {
	t.Parallel()
	id := mustUUID(t, pingIDText)
	var ev events.Event = events.SystemPingFlagged{V: 1, PingID: id}
	if ev.Type() != events.TypeSystemPingFlagged || ev.AggregateType() != "system" || ev.AggregateID() != id {
		t.Fatalf("SystemPingFlagged aggregate = %s %s %s", ev.Type(), ev.AggregateType(), ev.AggregateID())
	}
}
