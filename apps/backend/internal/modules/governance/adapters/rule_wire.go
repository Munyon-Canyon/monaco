package adapters

import (
	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/governanceapi"
)

type ruleUserWire struct {
	userID      uuid.UUID
	handle      *string
	displayName string
	photoURL    *string
}

func ruleParts(r *app.RuleView) (string, any, any, *ruleUserWire) {
	var user *ruleUserWire
	if r.User != nil {
		user = &ruleUserWire{
			userID: r.User.ID.UUID(), handle: present(r.User.Handle), displayName: r.User.DisplayName,
			photoURL: present(r.User.PhotoURL),
		}
	}
	return string(r.Change.Rule()), r.Change.From(), r.Change.To(), user
}

func wireRuleChange(r *app.RuleView) *api.ProposalRuleChange {
	if r == nil {
		return nil
	}
	rule, from, to, user := ruleParts(r)
	out := &api.ProposalRuleChange{Rule: api.ProposalRuleChangeRule(rule), From: from, To: to}
	if user != nil {
		out.User = &api.ProposalRuleChangeUser{
			UserId: user.userID, Handle: user.handle, DisplayName: user.displayName, PhotoUrl: user.photoURL,
		}
	}
	return out
}

func wireDetailRuleChange(r *app.RuleView) *api.ProposalDetailRuleChange {
	if r == nil {
		return nil
	}
	rule, from, to, user := ruleParts(r)
	out := &api.ProposalDetailRuleChange{Rule: api.ProposalDetailRuleChangeRule(rule), From: from, To: to}
	if user != nil {
		out.User = &api.ProposalDetailRuleChangeUser{
			UserId: user.userID, Handle: user.handle, DisplayName: user.displayName, PhotoUrl: user.photoURL,
		}
	}
	return out
}

func wirePendingRuleChange(r *app.RuleView) *api.PendingVoteRuleChange {
	if r == nil {
		return nil
	}
	rule, from, to, user := ruleParts(r)
	out := &api.PendingVoteRuleChange{Rule: api.PendingVoteRuleChangeRule(rule), From: from, To: to}
	if user != nil {
		out.User = &api.PendingVoteRuleChangeUser{
			UserId: user.userID, Handle: user.handle, DisplayName: user.displayName, PhotoUrl: user.photoURL,
		}
	}
	return out
}

func orEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func orZero(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
