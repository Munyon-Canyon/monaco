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
	name := current.Name
	if cmd.Name != nil {
		name = cmd.Name.String()
	}
	if cabal.Rules, err = cabal.Rules.Patch(cmd.Rules); err != nil {
		return err
	}
	changes := ruleChanges(current, name, cabal.Rules)
	if cmd.VoterIDs != nil {
		if changes.VoterIDs, err = voterChanges(ctx, q, cabal, current.ID, *cmd.VoterIDs); err != nil {
			return err
		}
	}
	if !changed(changes) {
		return nil
	}
	if err := writeEdit(ctx, q, current.ID, name, cabal.Rules, h.clock); err != nil {
		return err
	}
	if err := writeVoters(ctx, q, current, cabal.Rules, changes.VoterIDs); err != nil {
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
	if err != nil {
		return sqlc.FindCabalRow{}, domain.Cabal{}, errs.Wrap(err, errs.CodeInternal, updateCabalOp)
	}
	cabal := domain.Cabal{
		CreatorID: ids.UserIDFrom(row.CreatorID), Rules: rules, Banned: row.Status == string(port.StatusBanned),
	}
	return row, cabal, domain.CanEdit(actor, cabal)
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

func voterChanges(
	ctx context.Context, q *sqlc.Queries, cabal domain.Cabal, cabalID uuid.UUID, requested []ids.UserID,
) ([]uuid.UUID, error) {
	rows, err := q.ListMembers(ctx, cabalID)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, updateCabalOp)
	}
	members := make([]ids.UserID, 0, len(rows))
	var voters []uuid.UUID
	for _, row := range rows {
		members = append(members, ids.UserIDFrom(row.UserID))
		if row.CanVote {
			voters = append(voters, row.UserID)
		}
	}
	if err := domain.CheckVoters(cabal, members, requested); err != nil {
		return nil, err
	}
	next := make([]uuid.UUID, 0, len(requested))
	for _, voter := range requested {
		next = append(next, voter.UUID())
	}
	slices.SortFunc(next, compareUUID)
	next = slices.Compact(next)
	slices.SortFunc(voters, compareUUID)
	if slices.Equal(voters, next) {
		return nil, nil
	}
	return next, nil
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

func writeVoters(
	ctx context.Context, q *sqlc.Queries, current sqlc.FindCabalRow, rules domain.Rules, voters []uuid.UUID,
) error {
	var err error
	switch {
	case rules.VoterMode() == domain.VotersAll && current.VoterMode != string(domain.VotersAll):
		err = q.SetAllMembersVote(ctx, current.ID)
	case voters != nil:
		err = q.SetMemberVoters(ctx, sqlc.SetMemberVotersParams{VoterIds: voters, CabalID: current.ID})
	}
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, updateCabalOp)
	}
	return nil
}
