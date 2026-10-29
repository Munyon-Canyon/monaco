package app

import (
	"context"
	"encoding/json"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type RuleUser struct {
	ID          ids.UserID
	Handle      string
	DisplayName string
	PhotoURL    string
}

type RuleView struct {
	Change domain.RuleChange
	User   *RuleUser
}

func decodeRuleView(raw []byte) (view *RuleView, err error) {
	if len(raw) == 0 {
		return view, err
	}
	var change domain.RuleChange
	if err := json.Unmarshal(raw, &change); err != nil {
		return nil, errs.Wrap(err, errs.CodeDecodeFailed, "governance.RuleChange")
	}
	return &RuleView{Change: change}, nil
}

func (r *ProposalReads) nameRuleUsers(ctx context.Context, views []*RuleView) error {
	var who []ids.UserID
	for _, v := range views {
		if v != nil && v.Change.UserID() != nil {
			who = append(who, *v.Change.UserID())
		}
	}
	if len(who) == 0 {
		return nil
	}
	cards, err := r.users.UsersByID(ctx, who)
	if err != nil {
		return errs.Wrap(err, errs.CodeOf(err), "governance.RuleUsers")
	}
	for _, v := range views {
		if v == nil || v.Change.UserID() == nil {
			continue
		}
		id := *v.Change.UserID()
		card := cardOf(id, cards)
		v.User = &RuleUser{ID: id, Handle: cards[id].Handle, DisplayName: card.DisplayName, PhotoURL: card.PhotoURL}
	}
	return nil
}
