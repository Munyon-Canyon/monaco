package notify_test

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	pushHandler = "notify.notify_test_requested"
	opsActor    = "system:monacoctl"
)

type pushRig struct {
	pool   *pgxpool.Pool
	uow    *db.UnitOfWork
	ids    *testkit.IDs
	clock  *testkit.Clock
	sender *testkit.FakeSender
	users  app.Users
	logs   *testkit.Logs
}

func newPushRig(t *testing.T) *pushRig {
	t.Helper()
	pool := testkit.DB(t)
	g := testkit.NewIDs(7)
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	return &pushRig{
		pool: pool, uow: db.New(pool, g, clk), ids: g, clock: clk, sender: &testkit.FakeSender{},
		users: identity.New(module.Deps{Pool: pool}).Queries(), logs: &testkit.Logs{},
	}
}

func (r *pushRig) user(t *testing.T, status string) ids.UserID {
	t.Helper()
	return testkit.SeedUser(t, r.pool, testkit.UserOpts{AccountStatus: status}).ID
}

func (r *pushRig) device(t *testing.T, user ids.UserID, tok string) {
	t.Helper()
	r.exec(t, `INSERT INTO device_tokens (id, user_id, token, environment, created_at, last_seen_at)
		VALUES ($1, $2, $3, 'sandbox', $4, $4)`, r.ids.NewV7(), user.UUID(), tok, r.clock.Now())
}

func (r *pushRig) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := r.pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func (r *pushRig) trigger(t *testing.T, actor string, user ids.UserID) (bus.Delivery, events.NotifyTestRequested) {
	t.Helper()
	e := events.NotifyTestRequested{V: 1, UserID: user.UUID()}
	ctx := observability.WithActor(t.Context(), actor)
	if err := r.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error { return tx.Events.Append(ctx, e) }); err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	if err := r.pool.QueryRow(t.Context(), `SELECT id FROM events WHERE type = $1 ORDER BY id DESC LIMIT 1`,
		string(e.Type())).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return bus.Delivery{Handler: pushHandler, EventID: ids.EventIDFrom(id), At: r.clock.Now()}, e
}

func (r *pushRig) handle(
	t *testing.T, sender apns.Sender, d bus.Delivery, e events.NotifyTestRequested,
	kinds ...app.Kind[events.NotifyTestRequested],
) error {
	t.Helper()
	logger := observability.NewLogger(config.Config{Env: config.EnvTest}, r.logs)
	ctx := observability.WithLogger(observability.WithActor(t.Context(), "system:"+pushHandler), logger)
	pusher := app.NewPusher(r.uow, r.users, sender, r.ids, r.clock)
	return app.Notify[events.NotifyTestRequested]{Pusher: pusher, Kinds: kinds}.Handle(ctx, d, e)
}

func (r *pushRig) wantCount(t *testing.T, what string, want int, query string, args ...any) {
	t.Helper()
	var got int
	if err := r.pool.QueryRow(t.Context(), query, args...).Scan(&got); err != nil || got != want {
		t.Fatalf("%s = %d, %v, want %d", what, got, err, want)
	}
}

func (r *pushRig) wantRecorded(t *testing.T, d bus.Delivery, want int) {
	t.Helper()
	r.wantCount(t, "recorded deliveries", want,
		`SELECT count(*) FROM event_deliveries WHERE handler = $1 AND event_id = $2`, d.Handler, d.EventID.UUID())
}

func (r *pushRig) wantSentEvents(t *testing.T, d bus.Delivery, want int) {
	t.Helper()
	r.wantCount(t, "notification.sent events", want, `SELECT count(*) FROM events
		WHERE type = 'notification.sent' AND payload->>'source_event_id' = $1`, d.EventID.String())
}

func (r *pushRig) wantStates(t *testing.T, d bus.Delivery, want map[ids.UserID]string) {
	t.Helper()
	byID := make(map[string]string, len(want))
	for user, state := range want {
		byID[user.String()] = state
	}
	var got map[string]string
	if err := r.pool.QueryRow(t.Context(), `SELECT coalesce(jsonb_object_agg(user_id, state), '{}')
		FROM notifications WHERE source_event_id = $1`, d.EventID.UUID()).Scan(&got); err != nil ||
		!maps.Equal(got, byID) {
		t.Fatalf("notification states = %v, %v, want %v", got, err, byID)
	}
}

func (r *pushRig) wantDisabled(t *testing.T, tok string, want bool) {
	t.Helper()
	var at *time.Time
	if err := r.pool.QueryRow(t.Context(), `SELECT disabled_at FROM device_tokens WHERE token = $1`, tok).
		Scan(&at); err != nil {
		t.Fatal(err)
	}
	if (at != nil) != want || at != nil && !at.Equal(r.clock.Now()) {
		t.Fatalf("token %s disabled at %v, want disabled %v at %v", tok[:4], at, want, r.clock.Now())
	}
}

func wantVerdict(t *testing.T, err error, code errs.Code, verdict errs.Verdict) {
	t.Helper()
	got, v := errs.Code(""), errs.VerdictAck
	if err != nil {
		got, v = errs.CodeOf(err), errs.VerdictFor(errs.CodeOf(err))
	}
	if got != code || v != verdict {
		t.Fatalf("Handle = %v, want code %q with verdict %d", err, code, verdict)
	}
}

type crowd struct {
	users     []ids.UserID
	err       error
	renderErr error
	rendered  []ids.UserID
}

func (*crowd) Name() string { return "crowd" }

func (c *crowd) Recipients(context.Context, events.NotifyTestRequested) ([]ids.UserID, error) {
	return c.users, c.err
}

func (c *crowd) Render(ctx context.Context, e events.NotifyTestRequested, to ids.UserID) (app.Message, error) {
	c.rendered = append(c.rendered, to)
	if c.renderErr != nil {
		return app.Message{}, c.renderErr
	}
	return app.Test{}.Render(ctx, e, to)
}

type hooked struct {
	*testkit.FakeSender
	before func(ctx context.Context, p apns.Push) error
}

func (s hooked) Send(ctx context.Context, p apns.Push) (apns.Result, error) {
	if err := s.before(ctx, p); err != nil {
		return apns.Result{}, err
	}
	return s.FakeSender.Send(ctx, p)
}

func TestPush_deliversTheTestPushAppendsNotificationSentAndLogsWithoutTheToken(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	user := r.user(t, "active")
	r.device(t, user, token('a'))
	d, e := r.trigger(t, opsActor, user)

	wantVerdict(t, r.handle(t, r.sender, d, e, app.Test{}), "", errs.VerdictAck)

	want := apns.Push{
		UserID: user, Token: token('a'), Environment: apns.Sandbox, CollapseID: "test-" + user.String(),
		Title: "Monaco test", Body: "Push is working.",
		Data: map[string]string{"kind": "test", "user_id": user.String()},
	}
	if sent := r.sender.Sent(); !reflect.DeepEqual(sent, []apns.Push{want}) {
		t.Fatalf("sent %+v, want only %+v", sent, want)
	}
	r.wantStates(t, d, map[ids.UserID]string{user: "delivered"})
	r.wantCount(t, "rows delivered at the clock", 1, `SELECT count(*) FROM notifications
		WHERE delivered_at = $1 AND broadcast_id IS NULL AND kind = 'test'`, r.clock.Now())
	r.wantCount(t, "notification.sent naming the row", 1, `SELECT count(*) FROM events v
		JOIN notifications n ON v.payload->>'notification_id' = n.id::text
		WHERE v.type = 'notification.sent' AND v.aggregate_type = 'notification' AND v.aggregate_id = n.id
		AND v.actor_type = 'system' AND v.actor_id = $1 AND v.payload->>'user_id' = $2
		AND v.payload->>'kind' = 'test' AND v.payload->>'source_event_id' = $3`,
		pushHandler, user.String(), d.EventID.String())
	r.wantRecorded(t, d, 1)
	logs := string(r.logs.Bytes())
	if line := `"user_id":"` + user.String() + `","kind":"test","status":200,"reason":""`; !strings.Contains(logs,
		`"msg":"notify.push.result"`) || !strings.Contains(logs, line) || strings.Contains(logs, token('a')) {
		t.Fatalf("logs = %s, want notify.push.result with %s and never the token", logs, line)
	}
}

func TestPush_afterA429RedeliveryResendsOnlyThePendingRow(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	busy, free := r.user(t, "active"), r.user(t, "active")
	r.device(t, busy, token('a'))
	r.device(t, free, token('b'))
	r.sender.Reply(token('a'), apns.Result{Status: http.StatusTooManyRequests, RetryAfter: time.Minute})
	d, e := r.trigger(t, opsActor, busy)
	kind := &crowd{users: []ids.UserID{busy, free}}

	wantVerdict(t, r.handle(t, r.sender, d, e, kind), errs.CodeAPNSUnavailable, errs.VerdictNak)
	r.wantStates(t, d, map[ids.UserID]string{busy: "pending", free: "delivered"})
	r.wantRecorded(t, d, 0)

	r.sender.Reply(token('a'), apns.Result{Status: http.StatusOK})
	wantVerdict(t, r.handle(t, r.sender, d, e, kind), "", errs.VerdictAck)
	if sent := r.sender.Sent(); len(sent) != 3 || sent[2].Token != token('a') {
		t.Fatalf("sends = %+v, want a third push, to the pending row's token only", sent)
	}
	r.wantStates(t, d, map[ids.UserID]string{busy: "delivered", free: "delivered"})
	r.wantSentEvents(t, d, 2)
	r.wantCount(t, "rows in one broadcast of two", 2, `SELECT count(*) FROM notifications n
		JOIN notification_broadcasts b ON b.id = n.broadcast_id
		WHERE b.source_event_id = $1 AND b.recipient_count = 2 AND b.kind = 'crowd'`, d.EventID.UUID())
	r.wantCount(t, "broadcasts", 1, `SELECT count(*) FROM notification_broadcasts`)
	r.wantRecorded(t, d, 1)
}

func TestPush_settlesEachAnswerAndARedeliveryResendsOnlyPendingRows(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		replies  []apns.Result
		code     errs.Code
		verdict  errs.Verdict
		state    string
		disabled []bool
		resends  int
		recorded int
	}{
		"403 terms and stays pending": {
			[]apns.Result{{Status: http.StatusForbidden}},
			errs.CodeAPNSAuthFailed, errs.VerdictTerm, "pending",
			[]bool{false},
			1, 0,
		},
		"400 BadTopic is rejected, pending and acked": {
			[]apns.Result{{Status: http.StatusBadRequest, Reason: "BadTopic"}},
			"", errs.VerdictAck, "pending",
			[]bool{false},
			1, 1,
		},
		"410 with no other token ends no_device": {
			[]apns.Result{{Status: http.StatusGone, Reason: "Unregistered"}},
			"", errs.VerdictAck, "no_device",
			[]bool{true},
			0, 1,
		},
		"410 beside a live token delivers": {
			[]apns.Result{{Status: http.StatusGone}, {Status: http.StatusOK}},
			"", errs.VerdictAck, "delivered",
			[]bool{true, false},
			0, 1,
		},
		"410 beside a 429 disables the dead token and stays pending": {
			[]apns.Result{{Status: http.StatusGone}, {Status: http.StatusTooManyRequests}},
			errs.CodeAPNSUnavailable,
			errs.VerdictNak, "pending",
			[]bool{true, false},
			1, 0,
		},
		"no active token ends no_device": {nil, "", errs.VerdictAck, "no_device", nil, 0, 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			user := r.user(t, "active")
			for i, reply := range tc.replies {
				r.device(t, user, token(byte('a'+i)))
				r.sender.Reply(token(byte('a'+i)), reply)
			}
			d, e := r.trigger(t, opsActor, user)

			wantVerdict(t, r.handle(t, r.sender, d, e, app.Test{}), tc.code, tc.verdict)
			first := len(r.sender.Sent())
			wantVerdict(t, r.handle(t, r.sender, d, e, app.Test{}), tc.code, tc.verdict)

			if resent := len(r.sender.Sent()) - first; first != len(tc.replies) || resent != tc.resends {
				t.Fatalf("sends = %d then %d more, want %d then %d", first, resent, len(tc.replies), tc.resends)
			}
			r.wantStates(t, d, map[ids.UserID]string{user: tc.state})
			for i, disabled := range tc.disabled {
				r.wantDisabled(t, token(byte('a'+i)), disabled)
			}
			r.wantRecorded(t, d, tc.recorded)
		})
	}
}

func TestPush_foldsTheAnswersIntoOneErrorByPrecedence(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	denied, broken, busy := r.user(t, "active"), r.user(t, "active"), r.user(t, "active")
	r.device(t, denied, token('a'))
	r.device(t, broken, token('b'))
	r.device(t, busy, token('c'))
	refused := errs.New(errs.CodeInvalidInput, "test.send")
	failing := map[string]error{token('b'): refused}
	sender := hooked{
		FakeSender: r.sender,
		before:     func(_ context.Context, p apns.Push) error { return failing[p.Token] },
	}
	r.sender.Reply(token('a'), apns.Result{Status: http.StatusForbidden})
	r.sender.Reply(token('c'), apns.Result{Status: http.StatusServiceUnavailable})
	d, e := r.trigger(t, opsActor, denied)
	kind := &crowd{users: []ids.UserID{denied, broken, busy}}

	wantVerdict(t, r.handle(t, sender, d, e, kind), errs.CodeAPNSAuthFailed, errs.VerdictTerm)
	r.sender.Reply(token('a'), apns.Result{Status: http.StatusOK})
	if err := r.handle(t, sender, d, e, kind); !errors.Is(err, refused) {
		t.Fatalf("Handle = %v, want the send error itself, %v", err, refused)
	}
	delete(failing, token('b'))
	wantVerdict(t, r.handle(t, sender, d, e, kind), errs.CodeAPNSUnavailable, errs.VerdictNak)
	r.sender.Reply(token('c'), apns.Result{Status: http.StatusOK})
	wantVerdict(t, r.handle(t, sender, d, e, kind), "", errs.VerdictAck)
	r.wantStates(t, d, map[ids.UserID]string{denied: "delivered", broken: "delivered", busy: "delivered"})
	r.wantRecorded(t, d, 1)
}

func TestPush_skipsTheCausingDeletedAndUnknownUsers(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	causing, deleted, kept := r.user(t, "active"), r.user(t, "deleted"), r.user(t, "active")
	r.device(t, causing, token('a'))
	r.device(t, deleted, token('b'))
	r.device(t, kept, token('c'))
	d, e := r.trigger(t, "user:"+causing.String(), causing)
	kind := &crowd{users: []ids.UserID{causing, deleted, ids.NewUserID(r.ids), kept}}

	wantVerdict(t, r.handle(t, r.sender, d, e, kind), "", errs.VerdictAck)

	if sent := r.sender.Sent(); len(sent) != 1 || sent[0].Token != token('c') || !slices.Equal(kind.rendered,
		[]ids.UserID{kept}) {
		t.Fatalf("sends %+v and renders for %v, want one of each, for the kept user", sent, kind.rendered)
	}
	r.wantStates(t, d, map[ids.UserID]string{kept: "delivered"})
}

func TestPush_sendsOnlyAfterTheRowsCommit(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	first, second := r.user(t, "active"), r.user(t, "active")
	r.device(t, first, token('a'))
	r.device(t, second, token('b'))
	var (
		mu     sync.Mutex
		states []string
	)
	sender := hooked{FakeSender: r.sender, before: func(ctx context.Context, p apns.Push) error {
		var state string
		if err := r.pool.QueryRow(ctx, `SELECT state FROM notifications WHERE user_id = $1`, p.UserID.UUID()).
			Scan(&state); err != nil {
			state = err.Error()
		}
		mu.Lock()
		defer mu.Unlock()
		states = append(states, state)
		return nil
	}}
	d, e := r.trigger(t, opsActor, first)

	wantVerdict(t, r.handle(t, sender, d, e, &crowd{users: []ids.UserID{first, second}}), "", errs.VerdictAck)

	if !slices.Equal(states, []string{"pending", "pending"}) {
		t.Fatalf("states read from the pool during Send = %q, want pending twice", states)
	}
}

func TestNotify_returnsPortAndRenderFailuresAsIsBeforeWritingAnything(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeDBUnavailable, "test.port")
	for name, tc := range map[string]struct {
		users   app.Users
		kind    *crowd
		renders int
	}{
		"recipients": {nil, &crowd{err: down}, 0},
		"users":      {users{err: down}, &crowd{}, 0},
		"render":     {nil, &crowd{renderErr: down}, 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			causing, deleted, first := r.user(t, "active"), r.user(t, "deleted"), r.user(t, "active")
			r.device(t, first, token('a'))
			if tc.users != nil {
				r.users = tc.users
			}
			tc.kind.users = []ids.UserID{causing, deleted, first, r.user(t, "active")}
			d, e := r.trigger(t, "user:"+causing.String(), causing)

			err := r.handle(t, r.sender, d, e, tc.kind)

			if want := []ids.UserID{first}[:tc.renders]; !errors.Is(err, down) ||
				!slices.Equal(tc.kind.rendered, want) || len(r.sender.Sent()) != 0 {
				t.Fatalf("Handle = %v after rendering for %v and %d sends, want %v, renders for %v and none",
					err, tc.kind.rendered, len(r.sender.Sent()), down, want)
			}
			r.wantStates(t, d, map[ids.UserID]string{})
			r.wantRecorded(t, d, 0)
		})
	}
}

func TestPush_termsAnEventWithNoRowBeforeWritingOrSending(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	user := r.user(t, "active")
	r.device(t, user, token('a'))
	d := bus.Delivery{Handler: pushHandler, EventID: ids.EventIDFrom(r.ids.NewV7()), At: r.clock.Now()}

	err := r.handle(t, r.sender, d, events.NotifyTestRequested{V: 1, UserID: user.UUID()}, app.Test{})

	wantVerdict(t, err, errs.CodeInternal, errs.VerdictTerm)
	if sent := r.sender.Sent(); len(sent) != 0 {
		t.Fatalf("sent %+v for an event with no row, want nothing", sent)
	}
	r.wantStates(t, d, map[ids.UserID]string{})
	r.wantRecorded(t, d, 0)
}

func TestPush_returnsEachStoreFailureAndRecordsNothing(t *testing.T) {
	t.Parallel()
	ok, gone := apns.Result{Status: http.StatusOK}, apns.Result{Status: http.StatusGone}
	one := func(ids.UserID, ids.UserID) app.Kind[events.NotifyTestRequested] { return app.Test{} }
	two := func(user, other ids.UserID) app.Kind[events.NotifyTestRequested] {
		return &crowd{users: []ids.UserID{user, other}}
	}
	none := func(ids.UserID, ids.UserID) app.Kind[events.NotifyTestRequested] { return &crowd{} }
	for name, tc := range map[string]struct {
		arrange []string
		kind    func(user, other ids.UserID) app.Kind[events.NotifyTestRequested]
		reply   apns.Result
		want    errs.Code
	}{
		"event actor":  {[]string{`ALTER TABLE events RENAME TO gone`}, one, ok, errs.CodeDBUnavailable},
		"broadcast":    {[]string{`ALTER TABLE notification_broadcasts ADD CHECK (recipient_count < 2)`}, two, ok, errs.CodeInternal},
		"notification": {[]string{`ALTER TABLE notifications ADD CHECK (false)`}, one, ok, errs.CodeInternal},
		"undelivered":  {[]string{`ALTER TABLE notifications RENAME TO gone`}, none, ok, errs.CodeDBUnavailable},
		"tokens":       {[]string{`ALTER TABLE device_tokens RENAME TO gone`}, one, ok, errs.CodeDBUnavailable},
		"data": {[]string{`INSERT INTO notifications
			(id, user_id, kind, source_event_id, title, body, data, collapse_id, state, created_at)
			SELECT gen_random_uuid(), (payload->>'user_id')::uuid, 'test', id, 't', 'b', '"x"', 'c', 'pending', now()
			FROM events`}, one, ok, errs.CodeDecodeFailed},
		"disable":    {[]string{`ALTER TABLE device_tokens ADD CHECK (disabled_at IS NULL)`}, one, gone, errs.CodeInternal},
		"delivered":  {[]string{`ALTER TABLE notifications ADD CHECK (state <> 'delivered')`}, one, ok, errs.CodeInternal},
		"sent event": {[]string{`ALTER TABLE events ADD CHECK (type <> 'notification.sent')`}, one, ok, errs.CodeInternal},
		"no device": {
			[]string{`DELETE FROM device_tokens`, `ALTER TABLE notifications ADD CHECK (state <> 'no_device')`},
			one, ok, errs.CodeInternal,
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			user, other := r.user(t, "active"), r.user(t, "active")
			r.device(t, user, token('a'))
			r.sender.Reply(token('a'), tc.reply)
			d, e := r.trigger(t, opsActor, user)
			for _, sql := range tc.arrange {
				r.exec(t, sql)
			}

			if err := r.handle(t, r.sender, d, e, tc.kind(user, other)); errs.CodeOf(err) != tc.want {
				t.Fatalf("Handle = %v, want %s", err, tc.want)
			}
			r.wantCount(t, "recorded deliveries", 0, `SELECT count(*) FROM event_deliveries`)
		})
	}
}
