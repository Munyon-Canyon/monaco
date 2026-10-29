package notify_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	rankingport "github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const ledgerWeek = "2026-W42"

type ledgerStub struct {
	ledger rankingport.Ledger
	found  []bool
	err    error
	calls  int
}

func (s *ledgerStub) Ledger(context.Context, ids.UserID, string) (rankingport.Ledger, bool, error) {
	found := true
	if s.calls < len(s.found) {
		found = s.found[s.calls]
	}
	s.calls++
	return s.ledger, found && s.err == nil, s.err
}

func ledgerCall(kind, choice string, moveBps int) *rankingport.LedgerCall {
	return &rankingport.LedgerCall{
		ProposalID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("ledger-call")), Kind: kind, Symbol: "AMZNx",
		YourChoice: choice, MoveBps: moveBps, Right: true,
		DecidedAt: time.Date(2026, 10, 14, 15, 0, 0, 0, time.UTC),
	}
}

func bps(v int64) *int64 { return &v }

func ledgerUser() ids.UserID {
	return ids.UserIDFrom(uuid.NewSHA1(uuid.NameSpaceOID, []byte("ledger-user")))
}

func ledgerEvent(user ids.UserID) events.RankingLedgerReady {
	return events.RankingLedgerReady{V: 1, UserID: user.UUID(), Week: ledgerWeek}
}

func activeLedgerKind(stub *ledgerStub) app.WeeklyLedger {
	user := ledgerUser()
	cards := []identity.UserCard{{ID: user, AccountStatus: identity.AccountActive}}
	return app.WeeklyLedger{Users: fakes.NewIdentity(cards, nil), Ledgers: stub}
}

func TestNotify_WeeklyLedger_RendersTheCopyTable(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		ledger rankingport.Ledger
		body   string
	}{
		"a best call on a sell lean": {
			rankingport.Ledger{Voted: 3, Eligible: 3, Resolved: 2, BestCall: ledgerCall("buy", "no", 420)},
			"You voted 3 of 3. Best call: Sell AMZN, +4.2%.",
		},
		"a best call on a buy lean": {
			rankingport.Ledger{Voted: 2, Eligible: 4, Resolved: 1, BestCall: ledgerCall("buy", "yes", 1234)},
			"You voted 2 of 4. Best call: Buy AMZN, +12.3%.",
		},
		"a sell proposal voted yes is a sell": {
			rankingport.Ledger{Voted: 1, Eligible: 1, Resolved: 1, BestCall: ledgerCall("sell", "yes", 50)},
			"You voted 1 of 1. Best call: Sell AMZN, +0.5%.",
		},
		"a sell proposal voted no is a buy": {
			rankingport.Ledger{Voted: 1, Eligible: 1, Resolved: 1, BestCall: ledgerCall("sell", "no", 50)},
			"You voted 1 of 1. Best call: Buy AMZN, +0.5%.",
		},
		"resolved calls and none landed": {
			rankingport.Ledger{Voted: 2, Eligible: 3, Resolved: 2, StakeReturnBps: bps(900)},
			"You voted 2 of 3. No call landed this week.",
		},
		"a positive stake": {
			rankingport.Ledger{Voted: 1, Eligible: 3, StakeReturnBps: bps(150)},
			"Your stake +1.5% this week.",
		},
		"a zero stake": {
			rankingport.Ledger{Voted: 1, Eligible: 3, StakeReturnBps: bps(0)},
			"Your stake +0.0% this week.",
		},
		"a negative stake": {
			rankingport.Ledger{Voted: 1, Eligible: 3, StakeReturnBps: bps(-235)},
			"Your stake −2.4% this week.",
		},
		"no stake": {
			rankingport.Ledger{Voted: 0, Eligible: 3},
			"You voted 0 of 3.",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			user := ledgerUser()

			got, err := activeLedgerKind(&ledgerStub{ledger: tc.ledger}).Render(t.Context(), ledgerEvent(user), user)

			want := app.Message{
				Title: "The Ledger · Week 42", Body: tc.body,
				Data:       map[string]string{"kind": "weekly_ledger", "week": ledgerWeek},
				CollapseID: "ledger-" + ledgerWeek,
				Presentation: app.Presentation{
					ThreadID: "ledger", Category: "LEDGER", InterruptionLevel: app.InterruptionActive,
					Sender: app.Sender{Kind: app.SenderApp},
				},
			}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("Render = %+v, %v, want %+v", got, err, want)
			}
		})
	}
}

func TestNotify_WeeklyLedger_RenderFailuresAreReturned(t *testing.T) {
	t.Parallel()
	user := ledgerUser()
	down := errs.New(errs.CodeDBUnavailable, "test.ledgers")

	_, err := activeLedgerKind(&ledgerStub{err: down}).Render(t.Context(), ledgerEvent(user), user)
	if !errors.Is(err, down) {
		t.Fatalf("Render = %v, want the port error %v", err, down)
	}

	_, err = activeLedgerKind(&ledgerStub{found: []bool{false}}).Render(t.Context(), ledgerEvent(user), user)
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Render for a ledger that vanished = %v, want %q", err, errs.CodeInternal)
	}
}

func TestNotify_WeeklyLedger_RecipientsAreTheUserOnlyWithAnActiveAccountAndALedger(t *testing.T) {
	t.Parallel()
	user := ledgerUser()
	card := func(status identity.AccountStatus, deleted bool) []identity.UserCard {
		return []identity.UserCard{{ID: user, AccountStatus: status, Deleted: deleted}}
	}
	for name, tc := range map[string]struct {
		cards []identity.UserCard
		found bool
		want  []ids.UserID
	}{
		"active with a ledger":      {card(identity.AccountActive, false), true, []ids.UserID{user}},
		"active without a ledger":   {card(identity.AccountActive, false), false, nil},
		"unknown to the users port": {nil, true, nil},
		"deleted":                   {card(identity.AccountActive, true), true, nil},
		"suspended":                 {card(identity.AccountSuspended, false), true, nil},
		"banned":                    {card(identity.AccountBanned, false), true, nil},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			kind := app.WeeklyLedger{
				Users: fakes.NewIdentity(tc.cards, nil), Ledgers: &ledgerStub{found: []bool{tc.found}},
			}

			got, err := kind.Recipients(t.Context(), ledgerEvent(user))

			if err != nil || !slices.Equal(got, tc.want) {
				t.Fatalf("Recipients = %v, %v, want %v", got, err, tc.want)
			}
		})
	}
}

func TestNotify_WeeklyLedger_RecipientsReturnPortFailures(t *testing.T) {
	t.Parallel()
	user := ledgerUser()
	down := errs.New(errs.CodeDBUnavailable, "test.port")

	broken := app.WeeklyLedger{Users: users{err: down}, Ledgers: &ledgerStub{}}
	_, err := broken.Recipients(t.Context(), ledgerEvent(user))
	if !errors.Is(err, down) {
		t.Fatalf("Recipients with the users port down = %v, want %v", err, down)
	}

	_, err = activeLedgerKind(&ledgerStub{err: down}).Recipients(t.Context(), ledgerEvent(user))
	if !errors.Is(err, down) {
		t.Fatalf("Recipients with the ledgers port down = %v, want %v", err, down)
	}
}

func TestNotify_WeeklyLedger_PushesToTheUsersDevice(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	user := r.reachableUser(t, identity.AuthOnboardingCompleted, identity.AccountActive)
	e := ledgerEvent(user)
	d := r.emit(t, "system:poller.ranking.ledger", e)
	ledgers := &ledgerStub{ledger: rankingport.Ledger{
		Voted: 3, Eligible: 3, Resolved: 1, BestCall: ledgerCall("buy", "no", 420),
	}}

	err := handleKinds(t, r, r.sender, d, e, app.WeeklyLedger{Users: r.users, Ledgers: ledgers})

	wantVerdict(t, err, "", errs.VerdictAck)
	want := apns.Push{
		UserID: user, Token: token('a'), Environment: apns.Sandbox, CollapseID: "ledger-" + ledgerWeek,
		Title: "The Ledger · Week 42", Body: "You voted 3 of 3. Best call: Sell AMZN, +4.2%.",
		Data:     map[string]string{"kind": "weekly_ledger", "week": ledgerWeek, "sender_kind": "app"},
		ThreadID: "ledger", Category: "LEDGER", InterruptionLevel: "active", Sound: "default",
	}
	if sent := r.sender.Sent(); !reflect.DeepEqual(sent, []apns.Push{want}) {
		t.Fatalf("sent %+v, want only %+v", sent, want)
	}
	r.wantStates(t, d, map[ids.UserID]string{user: "delivered"})
}

func TestModule_WithLedgersFeedsTheWeeklyLedgerPush(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	user := r.reachableUser(t, identity.AuthOnboardingCompleted, identity.AccountActive)
	ledgers := &ledgerStub{ledger: rankingport.Ledger{Voted: 2, Eligible: 4, Resolved: 1}}
	deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock}

	d := r.dispatchTo(t, notify.New(deps, notify.WithSender(r.sender), notify.WithLedgers(ledgers)),
		"system:poller.ranking.weekly_ledger", ledgerEvent(user))

	if ledgers.calls == 0 || len(r.sender.Sent()) != 1 {
		t.Fatalf("ledger reads %d, pushes %d; want the option's port read and one push",
			ledgers.calls, len(r.sender.Sent()))
	}
	r.wantStates(t, d, map[ids.UserID]string{user: "delivered"})
}
