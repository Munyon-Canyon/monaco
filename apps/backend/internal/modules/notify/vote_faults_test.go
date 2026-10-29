package notify_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

var errPort = errs.New(errs.CodeDBUnavailable, "test.port")

type flakyStates struct {
	inner      app.VoteStates
	failState  int
	failDue    bool
	due        []governanceport.DueReminder
	afterNon   func()
	stateCalls int
}

func (f *flakyStates) VoteState(ctx context.Context, id ids.ProposalID) (governanceport.VoteState, error) {
	f.stateCalls++
	if f.failState > 0 && f.stateCalls >= f.failState {
		return governanceport.VoteState{}, errPort
	}
	return f.inner.VoteState(ctx, id)
}

func (f *flakyStates) DueForReminder(
	ctx context.Context, now time.Time, after governanceport.ReminderCursor, limit int,
) ([]governanceport.DueReminder, error) {
	if f.failDue {
		return nil, errPort
	}
	if f.due != nil {
		return f.due, nil
	}
	return f.inner.DueForReminder(ctx, now, after, limit)
}

func (f *flakyStates) NonVoters(ctx context.Context, id ids.ProposalID) ([]ids.UserID, error) {
	out, err := f.inner.NonVoters(ctx, id)
	if f.afterNon != nil {
		f.afterNon()
	}
	return out, err
}

type cancelingUsers struct {
	app.Users
	cancel context.CancelFunc
	err    error
}

func (u cancelingUsers) UsersByID(ctx context.Context, users []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	if u.err != nil {
		return nil, u.err
	}
	cards, err := u.Users.UsersByID(ctx, users)
	if u.cancel != nil {
		u.cancel()
	}
	return cards, err
}

func (w voteWorld) remindersOn(ports app.VotePorts, users app.Users) *app.RemindVoters {
	return app.NewRemindVoters(app.NewPusher(w.r.uow, users, w.r.sender, w.r.ids, w.r.clock), ports, 0)
}

func tickIn(ctx context.Context, p *app.RemindVoters) error {
	_, err := p.Tick(observability.WithActor(ctx, "system:poller."+p.Name()))
	return err
}

func wantTickError(ctx context.Context, t *testing.T, p *app.RemindVoters, why string) {
	t.Helper()
	if err := tickIn(ctx, p); err == nil {
		t.Fatalf("Tick = nil, want the %s failure", why)
	}
}

func TestRemindVoters_failsWhenAPortOrTheStoreFails(t *testing.T) {
	t.Parallel()
	type setup struct {
		prepare func(t *testing.T, w voteWorld, proposal string)
		states  func(w voteWorld, ctx context.CancelFunc) *flakyStates
		ports   func(p *app.VotePorts)
		users   func(w voteWorld, cancel context.CancelFunc) app.Users
	}
	cases := map[string]setup{
		"the votes of the proposal cannot be read": {
			states: func(w voteWorld, _ context.CancelFunc) *flakyStates {
				return &flakyStates{inner: w.ports.States, failState: 1}
			},
		},
		"the due list cannot be read": {
			states: func(w voteWorld, _ context.CancelFunc) *flakyStates {
				return &flakyStates{inner: w.ports.States, failDue: true}
			},
		},
		"the members cannot be read": {
			ports: func(p *app.VotePorts) { p.Cabals.(*fakes.Cabal).Fail("Members", errPort) },
		},
		"the cabal cannot be read": {
			ports: func(p *app.VotePorts) { p.Cabals.(*fakes.Cabal).Fail("Cabal", errPort) },
		},
		"the asset cannot be read": {
			ports: func(p *app.VotePorts) {
				assets := fixtureCatalog()
				assets.Fail("AssetBySymbol", errPort)
				p.Assets = assets
			},
		},
		"the proposal is neither a buy nor a sell": {
			prepare: func(t *testing.T, w voteWorld, proposal string) {
				t.Helper()
				w.r.exec(t, `UPDATE proposals SET kind = 'hold' WHERE id = $1`, proposal)
			},
		},
		"the users cannot be read": {
			users: func(w voteWorld, _ context.CancelFunc) app.Users {
				return cancelingUsers{Users: w.r.users, err: errPort}
			},
		},
		"the preferences cannot be read": {
			users: func(w voteWorld, cancel context.CancelFunc) app.Users {
				return cancelingUsers{Users: w.r.users, cancel: cancel}
			},
		},
		"the deciding rows cannot be read": {
			states: func(w voteWorld, cancel context.CancelFunc) *flakyStates {
				return &flakyStates{inner: w.ports.States, afterNon: cancel}
			},
		},
		"the notifications cannot be written": {
			prepare: func(t *testing.T, w voteWorld, _ string) {
				t.Helper()
				w.r.exec(t, `ALTER TABLE notifications ADD CONSTRAINT never CHECK (false) NOT VALID`)
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.voteWorldOf(t, 8)
			proposal := w.openAtOneThird(t)
			w.fixtureBallots(t, proposal)
			if tc.prepare != nil {
				tc.prepare(t, w, proposal.String())
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			ports := w.ports
			if tc.states != nil {
				ports.States = tc.states(w, cancel)
			}
			if tc.ports != nil {
				tc.ports(&ports)
			}
			users := r.users
			if tc.users != nil {
				users = tc.users(w, cancel)
			}

			wantTickError(ctx, t, w.remindersOn(ports, users), name)
		})
	}
}

func TestRemindVoters_failsWhenTheStoreCannotBeRead(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 8)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	wantTickError(ctx, t, w.reminders(), "cursor read")
}

func TestRemindVoters_retriesTheSendOfTheLastProposalBeforeTheNextBatch(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 8)
	proposal := w.openAtOneThird(t)
	w.fixtureBallots(t, proposal)
	p := w.reminders()
	down := errs.New(errs.CodeAPNSUnavailable, "test.apns")
	r.sender.Fail("Send", down)

	wantTickError(t.Context(), t, p, "send")
	wantTickError(t.Context(), t, p, "resend")
	r.sender.Fail("Send", nil)
	wantTick(t, p, 0, 0)

	r.wantCount(t, "delivered reminders", 3, `SELECT count(*) FROM notifications WHERE state = 'delivered'`)
	r.wantCount(t, "reminder rows", 3, `SELECT count(*) FROM notifications WHERE kind = 'vote_reminder'`)
}

func TestRemindVoters_ignoresAProposalThatClosedAfterItWasListed(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	w := r.voteWorldOf(t, 8)
	proposal := w.openAtOneThird(t)
	w.fixtureBallots(t, proposal)
	r.exec(t, `UPDATE proposals SET status = 'passed' WHERE id = $1`, proposal)
	ports := w.ports
	ports.States = &flakyStates{
		inner: w.ports.States,
		due: []governanceport.DueReminder{
			{ProposalID: ids.ProposalIDFrom(proposal), DueAt: r.clock.Now()},
		},
	}

	wantTick(t, w.remindersOn(ports, r.users), 1, 0)

	if len(r.sender.Sent()) != 0 {
		t.Fatalf("sent %+v, want nothing", r.sender.Sent())
	}
}

func TestVoteDeciding_failsWhenAPortFails(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		failState int
		kind      string
	}{
		"the first read":  {failState: 1},
		"the render read": {failState: 2},
		"the kind":        {kind: "hold"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			w := r.voteWorldOf(t, 8)
			proposal := w.proposeNow(t)
			w.castAll(t, proposal, map[int]string{0: "yes", 1: "yes", 2: "yes", 3: "no", 4: "yes"})
			if tc.kind != "" {
				r.exec(t, `UPDATE proposals SET kind = $2 WHERE id = $1`, proposal, tc.kind)
			}
			kind := w.deciding()
			kind.States = &flakyStates{inner: w.ports.States, failState: tc.failState}
			e := w.voteCast(proposal, w.members[4], "yes")
			d := r.emit(t, userActor(w.members[4]), e)

			if err := handleKinds(t, r, r.sender, d, e, kind); err == nil {
				t.Fatal("Handle = nil, want the port failure")
			}

			if len(r.sender.Sent()) != 0 {
				t.Fatalf("sent %+v, want nothing", r.sender.Sent())
			}
		})
	}
}
