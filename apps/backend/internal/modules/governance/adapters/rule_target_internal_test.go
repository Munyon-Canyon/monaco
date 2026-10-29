package adapters

import (
	"encoding/json"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
)

const targetUser = "019b76da-a802-77ac-83f3-e3b754873d7a"

func decodeChange(t *testing.T, raw string) api.RuleChangeRequestChange {
	t.Helper()
	var change api.RuleChangeRequestChange
	if err := json.Unmarshal([]byte(raw), &change); err != nil {
		t.Fatal(err)
	}
	return change
}

func TestRuleTarget_readsEachRulesValueShape(t *testing.T) {
	t.Parallel()
	cases := map[string]func(app.RuleTarget) bool{
		`{"rule":"show_sides","to":true}`:              func(g app.RuleTarget) bool { return g.Bool != nil && *g.Bool },
		`{"rule":"threshold","to":"unanimous"}`:        func(g app.RuleTarget) bool { return g.Text != nil && *g.Text == "unanimous" },
		`{"rule":"proposal_expiry_seconds","to":3600}`: func(g app.RuleTarget) bool { return g.Int != nil && *g.Int == 3600 },
		`{"rule":"max_trade_bps","to":null}`:           func(g app.RuleTarget) bool { return g.Int == nil },
		`{"rule":"max_trade_bps","to":500}`:            func(g app.RuleTarget) bool { return g.Int != nil && *g.Int == 500 },
		`{"rule":"voter","to":false,"user_id":"` + targetUser + `"}`: func(g app.RuleTarget) bool {
			return g.Bool != nil && !*g.Bool && g.UserID != nil && g.UserID.String() == targetUser
		},
	}
	for raw, ok := range cases {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			got, err := ruleTarget(decodeChange(t, raw))
			if err != nil || !ok(got) {
				t.Fatalf("ruleTarget(%s) = %+v, %v", raw, got, err)
			}
		})
	}
}

func TestRuleTarget_refusesTheWrongShape(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		`{"rule":"show_sides","to":"yes"}`,
		`{"rule":"admin","to":1}`,
		`{"rule":"join_mode","to":true}`,
		`{"rule":"max_trade_bps","to":"lots"}`,
		`{"rule":"proposal_expiry_seconds","to":1.5}`,
		`{"rule":"proposal_expiry_seconds","to":null}`,
		`{"rule":"proposal_expiry_seconds","to":1099511627776}`,
	} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			if _, err := ruleTarget(decodeChange(t, raw)); errs.CodeOf(err) != errs.CodeInvalidInput {
				t.Fatalf("ruleTarget(%s) = %v, want invalid_input", raw, err)
			}
		})
	}
}
