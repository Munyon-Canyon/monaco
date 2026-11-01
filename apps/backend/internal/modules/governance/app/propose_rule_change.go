package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type RuleCabals interface {
	Cabals
	Cabal(ctx context.Context, id ids.CabalID) (cabalport.CabalView, error)
	Admins(ctx context.Context, id ids.CabalID) ([]ids.UserID, error)
}

type RuleTarget struct {
	Rule   domain.Rule
	Bool   *bool
	Int    *int32
	Text   *string
	UserID *ids.UserID
}

type ProposeRuleChange struct {
	CabalID    ids.CabalID
	ProposerID ids.UserID
	Target     RuleTarget
	Thesis     string
}

type ProposeRuleChangeHandler struct {
	uow    Transactor
	ids    ids.Generator
	clock  clock.Clock
	cabals RuleCabals
	hints  Hints
}

func NewProposeRuleChangeHandler(
	uow Transactor, g ids.Generator, c clock.Clock, cabals RuleCabals, hints Hints,
) *ProposeRuleChangeHandler {
	return &ProposeRuleChangeHandler{uow: uow, ids: g, clock: c, cabals: cabals, hints: hints}
}

func (h *ProposeRuleChangeHandler) guard(ctx context.Context, cmd ProposeRuleChange) error {
	const op = "governance.ProposeRuleChange"
	if err := member(ctx, h.cabals, cmd.CabalID, cmd.ProposerID); err != nil {
		return err
	}
	if err := notBanned(ctx, h.cabals, cmd.CabalID, op); err != nil {
		return err
	}
	if !utf8.ValidString(cmd.Thesis) || utf8.RuneCountInString(cmd.Thesis) > domain.MaxThesisRunes {
		return errs.New(errs.CodeInvalidInput, op, slog.String("field", "thesis"))
	}
	return nil
}

func (h *ProposeRuleChangeHandler) Handle(ctx context.Context, cmd ProposeRuleChange) (ids.ProposalID, error) {
	const op = "governance.ProposeRuleChange"
	if err := h.guard(ctx, cmd); err != nil {
		return ids.ProposalID{}, err
	}
	rules, err := h.cabals.Rules(ctx, cmd.CabalID)
	if err != nil {
		return ids.ProposalID{}, err
	}
	rule, err := domain.ParseThresholdRule(string(rules.Threshold))
	if err != nil {
		return ids.ProposalID{}, err
	}
	voters, err := h.voters(ctx, cmd)
	if err != nil {
		return ids.ProposalID{}, err
	}
	change, err := h.change(ctx, cmd, rules, voters)
	if err != nil {
		return ids.ProposalID{}, err
	}
	raw, _ := json.Marshal(change)
	id := ids.ProposalIDFrom(h.ids.NewV7())
	now := h.clock.Now()
	params := sqlc.InsertRuleProposalParams{
		ID: id.UUID(), CabalID: cmd.CabalID.UUID(), ProposerID: cmd.ProposerID.UUID(), Thesis: cmd.Thesis,
		Threshold: string(rule), ExpiresAt: now.Add(rules.ProposalExpiry), CreatedAt: now,
		SidesAfterClose: rules.ShowSides, RuleChange: raw, VoterIds: make([]uuid.UUID, len(voters)),
	}
	for i, v := range voters {
		params.VoterIds[i] = v.UUID()
	}
	created := events.ProposalCreated{
		V: 1, ProposalID: id.UUID(), CabalID: cmd.CabalID.UUID(), ProposerID: cmd.ProposerID.UUID(),
		Kind: string(domain.KindRule), ExpiresAt: params.ExpiresAt, VoterCount: len(voters),
	}
	err = h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		if _, err := sqlc.New(tx.Queries()).InsertRuleProposal(ctx, params); err != nil {
			return insertFailed(err, op)
		}
		return tx.Events.Append(ctx, created)
	})
	if err != nil {
		return ids.ProposalID{}, err
	}
	h.hints.ProposalCreated(ctx, cmd.CabalID, id)
	return id, nil
}

func insertFailed(err error, op string) error {
	if db.IsUniqueViolation(err) {
		return errs.Wrap(err, errs.CodeRuleChangeOpen, op)
	}
	return errs.Wrap(err, errs.CodeInternal, op)
}

func (h *ProposeRuleChangeHandler) voters(ctx context.Context, cmd ProposeRuleChange) ([]ids.UserID, error) {
	const op = "governance.ProposeRuleChange"
	voters, err := h.cabals.VoterSet(ctx, cmd.CabalID)
	if err != nil {
		return nil, err
	}
	if len(voters) == 0 {
		return nil, errs.New(errs.CodeInvalidInput, op, slog.String("field", "voters"))
	}
	if !slices.Contains(voters, cmd.ProposerID) {
		return nil, errs.New(errs.CodeNotAVoter, op)
	}
	return voters, nil
}

func (h *ProposeRuleChangeHandler) change(
	ctx context.Context, cmd ProposeRuleChange, rules cabalport.Rules, voters []ids.UserID,
) (domain.RuleChange, error) {
	change, err := h.build(ctx, cmd, rules, voters)
	if err != nil {
		return domain.RuleChange{}, err
	}
	if change.Unchanged() != nil {
		return domain.RuleChange{}, errs.New(errs.CodeRuleUnchanged, "governance.ProposeRuleChange")
	}
	view, err := h.cabals.Cabal(ctx, cmd.CabalID)
	if err != nil {
		return domain.RuleChange{}, err
	}
	if err := change.NotCreator(view.CreatorID); err != nil {
		return domain.RuleChange{}, err
	}
	if change.Rule() != domain.RuleAdmin && change.Rule() != domain.RuleCreator {
		return change, nil
	}
	admins, err := h.cabals.Admins(ctx, cmd.CabalID)
	if err != nil {
		return domain.RuleChange{}, err
	}
	if change.Rule() == domain.RuleCreator {
		if err := change.ProposedByCreator(cmd.ProposerID, view.CreatorID); err != nil {
			return domain.RuleChange{}, err
		}
		return change, change.TargetIsAdmin(admins)
	}
	if err := change.KeepsAnAdmin(len(admins)); err != nil {
		return domain.RuleChange{}, err
	}
	return change, change.AdminIsVoter(slices.Contains(voters, *change.UserID()))
}

func (h *ProposeRuleChangeHandler) build(
	ctx context.Context, cmd ProposeRuleChange, rules cabalport.Rules, voters []ids.UserID,
) (domain.RuleChange, error) {
	t := cmd.Target
	switch t.Rule {
	case domain.RuleShowSides:
		to, err := wantBool(t)
		return domain.ShowSidesChange(rules.ShowSides, to), err
	case domain.RuleMaxTradeBps:
		return domain.MaxTradeBpsChange(rules.MaxTradeBps, t.Int)
	case domain.RuleThreshold:
		to, err := wantText(t)
		if err != nil {
			return domain.RuleChange{}, err
		}
		return domain.ThresholdChange(string(rules.Threshold), to)
	case domain.RuleExpiry:
		if t.Int == nil {
			return domain.RuleChange{}, missing(t.Rule)
		}
		return domain.ExpiryChange(int32(rules.ProposalExpiry.Seconds()), *t.Int)
	case domain.RuleJoinMode:
		to, err := wantText(t)
		if err != nil {
			return domain.RuleChange{}, err
		}
		return domain.JoinModeChange(string(rules.JoinMode), to)
	case domain.RuleVoter, domain.RuleAdmin:
		return h.memberChange(ctx, cmd, voters)
	case domain.RuleCreator:
		return creatorChange(t)
	}
	return domain.RuleChange{}, missing(t.Rule)
}

func (h *ProposeRuleChangeHandler) memberChange(
	ctx context.Context, cmd ProposeRuleChange, voters []ids.UserID,
) (domain.RuleChange, error) {
	t := cmd.Target
	to, err := wantBool(t)
	if err != nil {
		return domain.RuleChange{}, err
	}
	if t.UserID == nil {
		return domain.RuleChange{}, domain.ErrNotMemberRule
	}
	ok, err := h.cabals.IsMember(ctx, cmd.CabalID, *t.UserID)
	if err != nil {
		return domain.RuleChange{}, err
	}
	if !ok {
		return domain.RuleChange{}, domain.ErrNotMemberRule
	}
	if t.Rule == domain.RuleVoter {
		return domain.VoterChange(*t.UserID, slices.Contains(voters, *t.UserID), to), nil
	}
	admins, err := h.cabals.Admins(ctx, cmd.CabalID)
	if err != nil {
		return domain.RuleChange{}, err
	}
	return domain.AdminChange(*t.UserID, slices.Contains(admins, *t.UserID), to), nil
}

func creatorChange(t RuleTarget) (domain.RuleChange, error) {
	to, err := wantBool(t)
	if err != nil || !to {
		return domain.RuleChange{}, missing(t.Rule)
	}
	if t.UserID == nil {
		return domain.RuleChange{}, domain.ErrNotMemberRule
	}
	return domain.CreatorChange(*t.UserID), nil
}

func wantBool(t RuleTarget) (bool, error) {
	if t.Bool == nil {
		return false, missing(t.Rule)
	}
	return *t.Bool, nil
}

func wantText(t RuleTarget) (string, error) {
	if t.Text == nil {
		return "", missing(t.Rule)
	}
	return *t.Text, nil
}

func missing(rule domain.Rule) error {
	return errs.New(errs.CodeInvalidInput, "governance.ProposeRuleChange", slog.String("field", string(rule)))
}
