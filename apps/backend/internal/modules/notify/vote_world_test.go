package notify_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const (
	voteWorldMint   = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"
	deciderCategory = "VOTE_DECIDING"
	timeSensitive   = "time-sensitive"
)

type voteWorld struct {
	r       *pushRig
	cabalID ids.CabalID
	members []ids.UserID
	tokens  map[ids.UserID]string
	ports   app.VotePorts
}

func (r *pushRig) voteWorldOf(t *testing.T, voters int, leavers ...int) voteWorld {
	t.Helper()
	w := voteWorld{r: r, cabalID: ids.CabalIDFrom(r.ids.NewV7()), tokens: map[ids.UserID]string{}}
	var rows []fakes.CabalMember
	for i := range voters {
		user := r.user(t, "active")
		w.tokens[user] = token(byte('a' + i))
		r.device(t, user, w.tokens[user])
		w.members = append(w.members, user)
		if !contains(leavers, i) {
			rows = append(rows, fakes.CabalMember{CabalID: w.cabalID, Member: cabal.MemberView{
				UserID: user, Role: cabal.RoleMember, CanVote: true, JoinedAt: r.clock.Now(),
			}})
		}
	}
	seed := fakes.CabalSeed{View: cabal.View{ID: w.cabalID, Name: cabalName}}
	w.ports = app.VotePorts{
		Cabals: fakes.NewCabal([]fakes.CabalSeed{seed}, rows), Assets: fixtureCatalog(),
		States: governance.New(module.Deps{Pool: r.pool}).VoteStates(),
	}
	return w
}

func contains(xs []int, x int) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func (w voteWorld) propose(t *testing.T, created time.Time) uuid.UUID {
	t.Helper()
	id := w.r.ids.NewV7()
	w.r.exec(t, `INSERT INTO proposals (id, cabal_id, proposer_id, kind, symbol, mint, usdc_micros, quote_out_amount,
			threshold, status, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, 'buy', 'AAPLx', $4, $5, 105000000, 'majority', 'open', $6, $7, $7)`,
		id, w.cabalID.UUID(), w.members[0].UUID(), voteWorldMint, buyMicros, created.Add(voteWindow), created)
	for _, m := range w.members {
		w.r.exec(t, `INSERT INTO proposal_voters (proposal_id, voter_id) VALUES ($1, $2)`, id, m.UUID())
	}
	return id
}

func (w voteWorld) proposeNow(t *testing.T) uuid.UUID {
	t.Helper()
	return w.propose(t, w.r.clock.Now())
}

func (w voteWorld) cast(t *testing.T, proposal uuid.UUID, voter ids.UserID, choice string) {
	t.Helper()
	w.r.exec(t, `INSERT INTO votes (proposal_id, voter_id, choice, cast_at, first_cast_at) VALUES ($1, $2, $3, $4, $4)
		ON CONFLICT (proposal_id, voter_id) DO UPDATE SET choice = EXCLUDED.choice, cast_at = EXCLUDED.cast_at`,
		proposal, voter.UUID(), choice, w.r.clock.Now())
}

func (w voteWorld) castAll(t *testing.T, proposal uuid.UUID, choices map[int]string) {
	t.Helper()
	for i, choice := range choices {
		w.cast(t, proposal, w.members[i], choice)
	}
}

func (w voteWorld) voteCast(proposal uuid.UUID, voter ids.UserID, choice string) events.VoteCast {
	return events.VoteCast{
		V: 1, ProposalID: proposal, CabalID: w.cabalID.UUID(), VoterID: voter.UUID(), Choice: choice,
		CastAt: w.r.clock.Now(),
	}
}

func (w voteWorld) deciding() app.VoteDeciding {
	return app.VoteDeciding{VotePorts: w.ports, Clock: w.r.clock}
}

func (w voteWorld) reminders() *app.RemindVoters {
	return app.NewRemindVoters(app.NewPusher(w.r.uow, w.r.users, w.r.sender, w.r.ids, w.r.clock), w.ports, 0)
}

func (w voteWorld) pushTo(user ids.UserID, proposal uuid.UUID, title, body, category string) apns.Push {
	p := inCabal(apns.Push{
		UserID: user, Token: w.tokens[user], Environment: apns.Sandbox, CollapseID: "proposal-" + proposal.String(),
		Title: title, Body: body, Category: category,
		Data: map[string]string{
			"kind": kindOf(category), "cabal_id": w.cabalID.String(), "proposal_id": proposal.String(),
		},
	}, w.cabalID, cabalSender(w.cabalID))
	p.InterruptionLevel = timeSensitive
	return p
}

func kindOf(category string) string {
	if category == deciderCategory {
		return "vote_deciding"
	}
	return "vote_reminder"
}

type fixedVotes struct{ state governanceport.VoteState }

func (f fixedVotes) VoteState(context.Context, ids.ProposalID) (governanceport.VoteState, error) {
	return f.state, nil
}

func (fixedVotes) DueForReminder(
	context.Context, time.Time, governanceport.ReminderCursor, int,
) ([]governanceport.DueReminder, error) {
	return nil, nil
}

func (fixedVotes) NonVoters(context.Context, ids.ProposalID) ([]ids.UserID, error) { return nil, nil }

func decidingState(cabalID uuid.UUID) governanceport.VoteState {
	return governanceport.VoteState{
		CabalID:    ids.CabalIDFrom(cabalID),
		Kind:       buyKind,
		Symbol:     "AAPLx",
		Mint:       voteWorldMint,
		USDCMicros: money.MicrosFromUint64(buyMicros),
		Status:     governanceport.StatusOpen,
		Yes:        4,
		No:         1,
		Eligible:   8,
		Need:       5,
		CreatedAt: time.Date(
			2026,
			3,
			1,
			12,
			0,
			0,
			0,
			time.UTC,
		),
		ExpiresAt: time.Date(2026, 3, 1, 18, 0, 0, 0, time.UTC),
	}
}
