package app

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const updateCabalOp = "cabal.UpdateCabal"

type UpdateCabal struct {
	ActorID  ids.UserID
	CabalID  ids.CabalID
	Name     *domain.Name
	Rules    domain.RulesPatch
	VoterIDs *[]ids.UserID
}

type UpdateCabalHandler struct {
	uow   *db.UnitOfWork
	clock clock.Clock
}

func NewUpdateCabalHandler(uow *db.UnitOfWork, c clock.Clock) *UpdateCabalHandler {
	return &UpdateCabalHandler{uow: uow, clock: c}
}

func (h *UpdateCabalHandler) Handle(ctx context.Context, cmd UpdateCabal) error {
	return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return h.update(ctx, tx, cmd)
	})
}

func (h *UpdateCabalHandler) update(ctx context.Context, tx db.Tx, cmd UpdateCabal) error {
	q := sqlc.New(tx.Queries())
	current, cabal, err := lockForEdit(ctx, q, cmd.CabalID, cmd.ActorID)
	if err != nil {
		return err
	}
	if cmd.Rules.ChangesRules() || cmd.VoterIDs != nil {
		return errs.New(errs.CodeRulesChangeByProposal, updateCabalOp)
	}
	name := current.Name
	if cmd.Name != nil {
		name = cmd.Name.String()
	}
	if cabal.Rules, err = cabal.Rules.Patch(cmd.Rules); err != nil {
		return err
	}
	changes := ruleChanges(current, name, cabal.Rules)
	if !changed(changes) {
		return nil
	}
	if err := writeEdit(ctx, q, current.ID, name, cabal.Rules, h.clock); err != nil {
		return err
	}
	return tx.Events.Append(ctx, events.CabalUpdated{
		V: 1, CabalID: current.ID, ActorID: cmd.ActorID.UUID(), Changes: changes,
	})
}

func lockForEdit(
	ctx context.Context, q *sqlc.Queries, cabalID ids.CabalID, actor ids.UserID,
) (sqlc.FindCabalRow, domain.Cabal, error) {
	_, err := q.LockCabalExclusive(ctx, cabalID.UUID())
	if errors.Is(err, sql.ErrNoRows) {
		return sqlc.FindCabalRow{}, domain.Cabal{}, errs.New(errs.CodeCabalNotFound, updateCabalOp)
	}
	if err != nil {
		return sqlc.FindCabalRow{}, domain.Cabal{}, errs.Wrap(err, errs.CodeInternal, updateCabalOp)
	}
	return editable(ctx, q, cabalID, actor)
}

func editable(
	ctx context.Context, q *sqlc.Queries, cabalID ids.CabalID, actor ids.UserID,
) (sqlc.FindCabalRow, domain.Cabal, error) {
	row, cabal, err := loadCabal(ctx, q, cabalID)
	if err != nil {
		return sqlc.FindCabalRow{}, domain.Cabal{}, err
	}
	found, err := q.FindMember(ctx, sqlc.FindMemberParams{CabalID: cabalID.UUID(), UserID: actor.UUID()})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return sqlc.FindCabalRow{}, domain.Cabal{}, errs.Wrap(err, errs.CodeInternal, updateCabalOp)
	}
	if err == nil && domain.Role(found.Role) == domain.RoleCreator {
		cabal.Creators = []ids.UserID{actor}
	}
	member := domain.Actor{UserID: actor, Member: err == nil}
	return row, cabal, domain.CanEdit(member, cabal)
}

func loadCabal(ctx context.Context, q *sqlc.Queries, cabalID ids.CabalID) (sqlc.FindCabalRow, domain.Cabal, error) {
	row, err := q.FindCabal(ctx, cabalID.UUID())
	if errors.Is(err, sql.ErrNoRows) {
		return sqlc.FindCabalRow{}, domain.Cabal{}, errs.New(errs.CodeCabalNotFound, updateCabalOp)
	}
	if err != nil {
		return sqlc.FindCabalRow{}, domain.Cabal{}, errs.Wrap(err, errs.CodeInternal, updateCabalOp)
	}
	rules, err := domain.NewRules(
		row.JoinMode,
		row.VoterMode,
		row.Threshold,
		row.ProposalExpirySeconds,
		row.SlippageBps,
	)
	if err == nil {
		rules, err = storedRules(rules, row)
	}
	if err != nil {
		return sqlc.FindCabalRow{}, domain.Cabal{}, errs.Wrap(err, errs.CodeInternal, updateCabalOp)
	}
	cabal := domain.Cabal{
		Rules: rules, Banned: row.Status == string(port.StatusBanned),
	}
	return row, cabal, nil
}

func storedRules(rules domain.Rules, row sqlc.FindCabalRow) (domain.Rules, error) {
	return rules.WithShowSides(row.ShowSides, timePtr(row.ShowSidesChangedAt.Valid, row.ShowSidesChangedAt.Time)).
		WithMaxTradeBps(int32Ptr(row.MaxTradeBps.Valid, row.MaxTradeBps.Int32))
}

func ruleChanges(current sqlc.FindCabalRow, name string, rules domain.Rules) events.CabalChanges {
	return events.CabalChanges{
		Name:                  diff(current.Name, name),
		JoinMode:              diff(current.JoinMode, string(rules.JoinMode())),
		VoterMode:             diff(current.VoterMode, string(rules.VoterMode())),
		Threshold:             diff(current.Threshold, string(rules.Threshold())),
		ProposalExpirySeconds: diff(current.ProposalExpirySeconds, rules.ExpirySeconds()),
		SlippageBps:           diff(current.SlippageBps, rules.SlippageBps()),
	}
}

func diff[T comparable](current, next T) *T {
	if current == next {
		return nil
	}
	return &next
}

func changed(c events.CabalChanges) bool {
	return c.Name != nil || c.PictureURL != nil || c.JoinMode != nil || c.VoterMode != nil || c.VoterIDs != nil ||
		c.Threshold != nil || c.ProposalExpirySeconds != nil || c.SlippageBps != nil
}

func voterChanges(cabal domain.Cabal, rows []sqlc.ListMembersRow, requested []ids.UserID) []uuid.UUID {
	var voters []uuid.UUID
	for _, row := range rows {
		if row.CanVote {
			voters = append(voters, row.UserID)
		}
		if domain.Role(row.Role).IsAdmin() {
			cabal.Admins = append(cabal.Admins, ids.UserIDFrom(row.UserID))
		}
	}
	requested = cabal.VotersWithAdmins(requested)
	next := make([]uuid.UUID, 0, len(requested))
	for _, voter := range requested {
		next = append(next, voter.UUID())
	}
	slices.SortFunc(next, compareUUID)
	next = slices.Compact(next)
	slices.SortFunc(voters, compareUUID)
	if slices.Equal(voters, next) {
		return nil
	}
	return next
}

func compareUUID(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) }

func writeEdit(
	ctx context.Context, q *sqlc.Queries, cabalID uuid.UUID, name string, rules domain.Rules, c clock.Clock,
) error {
	n, err := q.UpdateCabal(ctx, sqlc.UpdateCabalParams{
		Name: name, JoinMode: string(rules.JoinMode()), VoterMode: string(rules.VoterMode()),
		Threshold: string(rules.Threshold()), ProposalExpirySeconds: rules.ExpirySeconds(),
		SlippageBps: rules.SlippageBps(), Now: c.Now(), ID: cabalID,
	})
	if err != nil || n != 1 {
		return errs.Wrap(err, errs.CodeInternal, updateCabalOp)
	}
	return nil
}

func writeVoters(ctx context.Context, q *sqlc.Queries, cabalID uuid.UUID, voters []uuid.UUID) error {
	if err := q.SetMemberVoters(ctx, sqlc.SetMemberVotersParams{VoterIds: voters, CabalID: cabalID}); err != nil {
		return errs.Wrap(err, errs.CodeInternal, updateCabalOp)
	}
	return nil
}
