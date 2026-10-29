package adapters

import (
	"encoding/json"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func ruleTarget(c api.RuleChangeRequestChange) (app.RuleTarget, error) {
	rule := domain.Rule(c.Rule)
	target := app.RuleTarget{Rule: rule}
	switch rule {
	case domain.RuleShowSides, domain.RuleVoter, domain.RuleAdmin:
		to, ok := c.To.(bool)
		if !ok {
			return target, badRuleValue(rule)
		}
		target.Bool = &to
	case domain.RuleThreshold, domain.RuleJoinMode:
		to, ok := c.To.(string)
		if !ok {
			return target, badRuleValue(rule)
		}
		target.Text = &to
	case domain.RuleMaxTradeBps, domain.RuleExpiry:
		if c.To == nil && rule == domain.RuleMaxTradeBps {
			break
		}
		to, ok := wholeNumber(c.To)
		if !ok {
			return target, badRuleValue(rule)
		}
		target.Int = &to
	}
	if c.UserId != nil {
		user := ids.UserIDFrom(*c.UserId)
		target.UserID = &user
	}
	return target, nil
}

func wholeNumber(v any) (int32, bool) {
	if v == nil {
		return 0, false
	}
	raw, _ := json.Marshal(v)
	var n int32
	return n, json.Unmarshal(raw, &n) == nil
}

func badRuleValue(rule domain.Rule) error {
	return errs.New(errs.CodeInvalidInput, "governance.RuleTarget", slog.String("field", string(rule)))
}
