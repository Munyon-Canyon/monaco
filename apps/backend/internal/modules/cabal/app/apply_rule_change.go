package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	applyRuleChangeOp = "cabal.ApplyRuleChange"
	passedRuleVersion = 1

	SkipCabalClosed     ruleSkip = "CabalClosed"
	SkipMemberGone      ruleSkip = "MemberGone"
	SkipInvariantBroken ruleSkip = "InvariantBroken"

	ruleShowSides   = "show_sides"
	ruleMaxTradeBps = "max_trade_bps"
	ruleThreshold   = "threshold"
	ruleExpiry      = "proposal_expiry_seconds"
	ruleJoinMode    = "join_mode"
	ruleVoter       = "voter"
	ruleAdmin       = "admin"
)

type PassedRule struct {
	Rule    string
	On      bool
	Text    string
	Seconds int32
	Bps     *int32
	UserID  ids.UserID
}

type passedRuleWire struct {
	V      int             `json:"v"`
	Rule   string          `json:"rule"`
	To     json.RawMessage `json:"to"`
	UserID *uuid.UUID      `json:"user_id"`
}

func ParsePassedRule(raw json.RawMessage) (PassedRule, error) {
	var wire passedRuleWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return PassedRule{}, errs.Wrap(err, errs.CodeDecodeFailed, applyRuleChangeOp)
	}
	if wire.V != passedRuleVersion {
		return PassedRule{}, errs.New(errs.CodeDecodeFailed, applyRuleChangeOp, slog.Int("v", wire.V))
	}
	rule := PassedRule{Rule: wire.Rule}
	if wire.UserID != nil {
		rule.UserID = ids.UserIDFrom(*wire.UserID)
	}
	var target any
	switch wire.Rule {
	case ruleShowSides, ruleVoter, ruleAdmin:
		target = &rule.On
	case ruleThreshold, ruleJoinMode:
		target = &rule.Text
	case ruleExpiry:
		target = &rule.Seconds
	case ruleMaxTradeBps:
		target = &rule.Bps
	default:
		return PassedRule{}, errs.New(errs.CodeDecodeFailed, applyRuleChangeOp, slog.String("rule", wire.Rule))
	}
	dec := json.NewDecoder(bytes.NewReader(wire.To))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return PassedRule{}, errs.Wrap(err, errs.CodeDecodeFailed, applyRuleChangeOp)
	}
	return rule, nil
}

type ApplyRuleChange struct {
	ProposalID ids.ProposalID
	CabalID    ids.CabalID
	ProposerID ids.UserID
	Change     PassedRule
	PassedAt   time.Time
}

type ApplyRuleChangeHandler struct{}

type ruleSkip string

func (h ApplyRuleChangeHandler) Apply(ctx context.Context, tx db.Tx, in ApplyRuleChange, at time.Time) error {
	skip, err := h.apply(ctx, tx, in, at)
	if err != nil {
		return err
	}
	if skip != "" {
		observability.Degraded(ctx, observability.CabalRuleChangeSkipped,
			slog.String("proposal_id", in.ProposalID.String()), slog.String("reason", string(skip)))
	}
	return nil
}

func (ApplyRuleChangeHandler) apply(ctx context.Context, tx db.Tx, in ApplyRuleChange, _ time.Time) (ruleSkip, error) {
	q := sqlc.New(tx.Queries())
	if _, err := q.LockCabalExclusive(ctx, in.CabalID.UUID()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SkipCabalClosed, nil
		}
		return "", errs.Wrap(err, errs.CodeInternal, applyRuleChangeOp)
	}
	if _, err := q.LockMembers(ctx, in.CabalID.UUID()); err != nil {
		return "", errs.Wrap(err, errs.CodeInternal, applyRuleChangeOp)
	}
	current, cabal, err := loadCabal(ctx, q, in.CabalID)
	if err != nil {
		return "", err
	}
	if cabal.Banned {
		return SkipCabalClosed, nil
	}
	members, err := q.ListMembers(ctx, in.CabalID.UUID())
	if err != nil {
		return "", errs.Wrap(err, errs.CodeInternal, applyRuleChangeOp)
	}
	next := ruleApplication{in: in, current: current, cabal: cabal, members: members, q: q}
	if skip := next.run(); skip != "" {
		return skip, nil
	}
	if err := next.write(ctx, tx); err != nil {
		return "", err
	}
	return "", nil
}

type ruleApplication struct {
	in      ApplyRuleChange
	current sqlc.FindCabalRow
	cabal   domain.Cabal
	members []sqlc.ListMembersRow
	q       *sqlc.Queries
	changes events.CabalChanges
	voters  []uuid.UUID
	role    *string
	target  ids.UserID
}

func (a *ruleApplication) run() ruleSkip {
	rule := a.in.Change
	switch rule.Rule {
	case ruleShowSides:
		return a.showSides()
	case ruleMaxTradeBps:
		return a.maxTradeBps()
	case ruleThreshold, ruleExpiry, ruleJoinMode:
		return a.plain()
	}
	member, ok := a.member(rule.UserID)
	switch {
	case !ok:
		return SkipMemberGone
	case domain.Role(member.Role) == domain.RoleCreator:
		return SkipInvariantBroken
	}
	a.target = rule.UserID
	if rule.Rule == ruleAdmin {
		return a.admin(member)
	}
	return a.voter(member)
}

func (a *ruleApplication) member(user ids.UserID) (sqlc.ListMembersRow, bool) {
	for _, m := range a.members {
		if m.UserID == user.UUID() {
			return m, true
		}
	}
	return sqlc.ListMembersRow{}, false
}

func (a *ruleApplication) showSides() ruleSkip {
	on := a.in.Change.On
	if a.cabal.Rules.ShowSides() == on {
		return SkipInvariantBroken
	}
	at := a.in.PassedAt
	a.cabal.Rules = a.cabal.Rules.WithShowSides(on, &at)
	a.changes.ShowSides = &on
	return ""
}

func (a *ruleApplication) maxTradeBps() ruleSkip {
	bps := a.in.Change.Bps
	rules, err := a.cabal.Rules.WithMaxTradeBps(bps)
	same := (bps == nil && a.cabal.Rules.MaxTradeBps() == nil) ||
		(bps != nil && a.cabal.Rules.MaxTradeBps() != nil && *bps == *a.cabal.Rules.MaxTradeBps())
	if err != nil || same {
		return SkipInvariantBroken
	}
	a.cabal.Rules = rules
	a.changes.MaxTradeBps, a.changes.MaxTradeBpsCleared = bps, bps == nil
	return ""
}

func (a *ruleApplication) plain() ruleSkip {
	rule := a.in.Change
	var patch domain.RulesPatch
	switch rule.Rule {
	case ruleThreshold:
		patch.Threshold = &rule.Text
	case ruleJoinMode:
		patch.JoinMode = &rule.Text
	default:
		patch.ExpirySeconds = &rule.Seconds
	}
	rules, err := a.cabal.Rules.Patch(patch)
	if err != nil {
		return SkipInvariantBroken
	}
	changes := ruleChanges(a.current, a.current.Name, rules)
	if !changed(changes) {
		return SkipInvariantBroken
	}
	a.cabal.Rules, a.changes = rules, changes
	return ""
}

func (a *ruleApplication) admin(member sqlc.ListMembersRow) ruleSkip {
	on := a.in.Change.On
	if domain.Role(member.Role).IsAdmin() == on {
		return SkipInvariantBroken
	}
	role := string(domain.RoleMember)
	if on {
		role = string(domain.RoleAdmin)
	}
	a.role = &role
	var admins []uuid.UUID
	for _, m := range a.members {
		isAdmin := domain.Role(m.Role).IsAdmin()
		if m.UserID == member.UserID {
			isAdmin = on
		}
		if isAdmin {
			admins = append(admins, m.UserID)
		}
	}
	slices.SortFunc(admins, compareUUID)
	a.changes.AdminIDs = admins
	return ""
}

func (a *ruleApplication) voter(member sqlc.ListMembersRow) ruleSkip {
	on := a.in.Change.On
	if domain.Role(member.Role).IsAdmin() && !on {
		return SkipInvariantBroken
	}
	requested := make([]ids.UserID, 0, len(a.members))
	for _, m := range a.members {
		if (m.CanVote && m.UserID != member.UserID) || (m.UserID == member.UserID && on) {
			requested = append(requested, ids.UserIDFrom(m.UserID))
		}
	}
	rules := a.cabal.Rules.WithVoterList()
	a.cabal.Rules = rules
	voters := voterChanges(a.cabal, a.members, requested)
	if voters == nil {
		return SkipInvariantBroken
	}
	a.voters = voters
	a.changes.VoterMode = diff(a.current.VoterMode, string(rules.VoterMode()))
	a.changes.VoterIDs = voters
	return ""
}

func (a *ruleApplication) write(ctx context.Context, tx db.Tx) error {
	rules := a.cabal.Rules
	row := sqlc.UpdateCabalRulesParams{
		JoinMode: string(rules.JoinMode()), VoterMode: string(rules.VoterMode()),
		Threshold: string(rules.Threshold()), ProposalExpirySeconds: rules.ExpirySeconds(),
		ShowSides: rules.ShowSides(), Now: a.in.PassedAt, ID: a.current.ID,
	}
	if at := rules.ShowSidesChangedAt(); at != nil {
		row.ShowSidesChangedAt.Time, row.ShowSidesChangedAt.Valid = *at, true
	}
	if bps := rules.MaxTradeBps(); bps != nil {
		row.MaxTradeBps.Int32, row.MaxTradeBps.Valid = *bps, true
	}
	n, err := a.q.UpdateCabalRules(ctx, row)
	if err != nil || n != 1 {
		return errs.Wrap(err, errs.CodeInternal, applyRuleChangeOp)
	}
	if a.voters != nil {
		if err := writeVoters(ctx, a.q, a.current.ID, a.voters); err != nil {
			return err
		}
	}
	if a.role != nil {
		n, err := a.q.SetMemberRole(ctx, sqlc.SetMemberRoleParams{
			Role: *a.role, CanVote: true, CabalID: a.current.ID, UserID: a.target.UUID(),
		})
		if err != nil || n != 1 {
			return errs.Wrap(err, errs.CodeInternal, applyRuleChangeOp)
		}
	}
	proposal := a.in.ProposalID.UUID()
	a.changes.ProposalID = &proposal
	return tx.Events.Append(ctx, events.CabalUpdated{
		V: 1, CabalID: a.current.ID, ActorID: a.in.ProposerID.UUID(), Changes: a.changes,
	})
}
