package app

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	voteReminderInterval = time.Minute
	voteReminderBatch    = 50
	voteReminderGrace    = 2 * time.Minute
	voteReminderPoller   = "notify.vote_reminders"
	voteReminderCursor   = "vote_reminders"
	opVoteReminder       = "notify.VoteReminder"

	voteDecidingKind = "vote_deciding"
)

type VoteStates = governanceport.VoteStates

type VotePorts struct {
	Cabals Cabals
	Assets Assets
	States VoteStates
}

func (v VotePorts) nonVoters(ctx context.Context, proposal ids.ProposalID, cabalID ids.CabalID) ([]ids.UserID, error) {
	waiting, err := v.States.NonVoters(ctx, proposal)
	if err != nil || len(waiting) == 0 {
		return nil, err
	}
	raw := cabalID.UUID()
	members, err := memberIDs(ctx, v.Cabals, &raw)
	if err != nil {
		return nil, err
	}
	in := make(map[ids.UserID]bool, len(members))
	for _, m := range members {
		in[m] = true
	}
	var out []ids.UserID
	for _, u := range waiting {
		if in[u] {
			out = append(out, u)
		}
	}
	return out, nil
}

type voteCopy struct {
	action, asset, amount string
}

func (v VotePorts) copyOf(ctx context.Context, state governanceport.VoteState) (voteCopy, error) {
	action, err := side(state.Kind)
	if err != nil {
		return voteCopy{}, err
	}
	asset, err := labelOf(ctx, v.Assets, state.Symbol)
	if err != nil {
		return voteCopy{}, err
	}
	return voteCopy{action: action, asset: asset, amount: dollars(state.USDCMicros)}, nil
}

func (v VotePorts) message(
	ctx context.Context, kind, category string, proposal ids.ProposalID, state governanceport.VoteState,
	title, body string,
) (Message, error) {
	view, err := v.Cabals.Cabal(ctx, state.CabalID)
	if err != nil {
		return Message{}, err
	}
	pres := ForCabal(view, CabalSender(view))
	pres.Category, pres.InterruptionLevel = category, InterruptionTimeSensitive
	return proposalMessage(kind, state.CabalID.UUID(), proposal.UUID(), cmp.Or(title, view.Name), body, pres), nil
}

func reminderSource(proposal ids.ProposalID) ids.EventID {
	return ids.EventIDFrom(uuid.NewSHA1(uuid.NameSpaceOID, []byte("reminder:"+proposal.String())))
}

func decidingSource(proposal uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("deciding:"+proposal.String()))
}

type voteReminder struct{}

func (voteReminder) Name() string { return "vote_reminder" }

type RemindVoters struct {
	pusher *Pusher
	ports  VotePorts
	every  time.Duration
}

func NewRemindVoters(p *Pusher, ports VotePorts, every time.Duration) *RemindVoters {
	return &RemindVoters{pusher: p, ports: ports, every: every}
}

func (*RemindVoters) Name() string { return voteReminderPoller }

func (p *RemindVoters) Interval() time.Duration {
	if p.every > 0 {
		return p.every
	}
	return voteReminderInterval
}

func (p *RemindVoters) Tick(ctx context.Context) (poller.Report, error) {
	now := p.pusher.clock.Now()
	cursor, found, err := p.cursor(ctx)
	if err != nil {
		return poller.Report{}, err
	}
	if found {
		if err := p.pusher.send(ctx, reminderSource(cursor.ProposalID)); err != nil {
			return poller.Report{}, err
		}
	}
	due, err := p.ports.States.DueForReminder(ctx, now, cursor, voteReminderBatch)
	if err != nil {
		return poller.Report{}, err
	}
	report := poller.Report{Scanned: len(due)}
	for _, d := range due {
		written, err := p.remind(ctx, now, d)
		report.Changed += written
		if err != nil {
			return report, err
		}
	}
	return report, nil
}

func (p *RemindVoters) cursor(ctx context.Context) (governanceport.ReminderCursor, bool, error) {
	row, err := sqlc.New(p.pusher.uow.Reads()).GetReminderCursor(ctx, voteReminderCursor)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return governanceport.ReminderCursor{}, false, nil
	case err != nil:
		return governanceport.ReminderCursor{}, false, errs.Wrap(err, errs.CodeDBUnavailable, opVoteReminder)
	}
	return governanceport.ReminderCursor{
		DueAt:      row.DueAt.UTC(),
		ProposalID: ids.ProposalIDFrom(row.ProposalID),
	}, true, nil
}

func (p *RemindVoters) remind(ctx context.Context, now time.Time, due governanceport.DueReminder) (int, error) {
	advance := func(ctx context.Context, q *sqlc.Queries) error {
		return q.AdvanceReminderCursor(ctx, sqlc.AdvanceReminderCursorParams{
			Name: voteReminderCursor, DueAt: due.DueAt, ProposalID: due.ProposalID.UUID(),
		})
	}
	source := reminderSource(due.ProposalID)
	delivery := bus.Delivery{Handler: voteReminderPoller, EventID: source, At: now}
	notes, err := p.notes(ctx, now, due)
	if err != nil {
		return 0, err
	}
	written, err := p.pusher.writeWith(ctx, delivery, notes, advance)
	if err != nil {
		return 0, err
	}
	return written, p.pusher.send(ctx, source)
}

func (p *RemindVoters) notes(
	ctx context.Context,
	now time.Time,
	due governanceport.DueReminder,
) ([]Notification, error) {
	state, err := p.ports.States.VoteState(ctx, due.ProposalID)
	if err != nil {
		return nil, err
	}
	if state.Status != governanceport.StatusOpen || state.Kind == ruleKind || !now.Before(state.ExpiresAt) {
		return nil, nil
	}
	to, err := p.recipients(ctx, due.ProposalID, state)
	if err != nil || len(to) == 0 {
		return nil, err
	}
	facts, err := p.ports.copyOf(ctx, state)
	if err != nil {
		return nil, err
	}
	ref := now
	if now.Sub(due.DueAt) < voteReminderGrace {
		ref = due.DueAt
	}
	title := reminderTitle(state.Eligible-state.Yes-state.No, closesIn(state.ExpiresAt.Sub(ref)))
	body := reminderBody(facts.action, facts.asset, facts.amount, state.Yes, state.No)
	msg, err := p.ports.message(ctx, voteReminder{}.Name(), voteCategory, due.ProposalID, state, title, body)
	if err != nil {
		return nil, err
	}
	source := reminderSource(due.ProposalID)
	notes := make([]Notification, len(to))
	for i, user := range to {
		notes[i] = Notification{Kind: voteReminder{}.Name(), UserID: user, Message: msg, source: source}
	}
	return notes, nil
}

func (p *RemindVoters) recipients(
	ctx context.Context, proposal ids.ProposalID, state governanceport.VoteState,
) ([]ids.UserID, error) {
	waiting, err := p.ports.nonVoters(ctx, proposal, state.CabalID)
	if err != nil || len(waiting) == 0 {
		return nil, err
	}
	told, err := sqlc.New(p.pusher.uow.Reads()).UsersWithNotification(ctx, sqlc.UsersWithNotificationParams{
		SourceEventID: decidingSource(proposal.UUID()), Kind: voteDecidingKind,
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, opVoteReminder)
	}
	skip := make(map[ids.UserID]bool, len(told))
	for _, u := range told {
		skip[ids.UserIDFrom(u)] = true
	}
	var open []ids.UserID
	for _, u := range waiting {
		if !skip[u] {
			open = append(open, u)
		}
	}
	return p.pusher.wanted(ctx, voteReminder{}.Name(), open)
}
