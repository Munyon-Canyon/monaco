package notify_test

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/apns"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func utc(day, hour, minute int) time.Time {
	return time.Date(2026, 3, day, hour, minute, 0, 0, time.UTC)
}

func (r *pushRig) follower(t *testing.T, handle, displayName string) ids.UserID {
	t.Helper()
	user := testkit.SeedUser(t, r.pool, testkit.UserOpts{Handle: handle}).ID
	r.exec(t, `UPDATE users SET display_name = $2 WHERE id = $1`, user.UUID(), displayName)
	return user
}

func (r *pushRig) followed(t *testing.T, followee, follower ids.UserID) (bus.Delivery, events.FollowCreated) {
	t.Helper()
	e := goldenEvent(t, events.TypeFollowCreated).(events.FollowCreated)
	e.FollowID, e.FolloweeID, e.FollowerID, e.CreatedAt = r.ids.NewV7(), followee.UUID(), follower.UUID(), r.clock.Now()
	return r.emit(t, userActor(follower), e), e
}

func (r *pushRig) handleFollow(t *testing.T, d bus.Delivery, e events.FollowCreated) error {
	t.Helper()
	return handleKinds(t, r, r.sender, d, e, app.NewFollower{Users: r.users})
}

func (r *pushRig) followDigest(opts ...notify.Option) *app.FollowDigest {
	deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock}
	return notify.New(deps, append([]notify.Option{notify.WithSender(r.sender)}, opts...)...).
		Pollers()[0].(*app.FollowDigest)
}

func tick(t *testing.T, p poller.Poller) (poller.Report, error) {
	t.Helper()
	return p.Tick(observability.WithActor(t.Context(), "system:poller."+p.Name()))
}

func wantTick(t *testing.T, p poller.Poller, scanned, changed int) {
	t.Helper()
	if got, err := tick(t, p); err != nil || got.Scanned != scanned || got.Changed != changed {
		t.Fatalf("Tick = %+v, %v, want scanned %d and changed %d", got, err, scanned, changed)
	}
}

func (r *pushRig) batched(t *testing.T, user ids.UserID, at time.Time, n int) {
	t.Helper()
	r.exec(t, `INSERT INTO notifications
		(id, user_id, kind, source_event_id, title, body, data, collapse_id, state, created_at)
		SELECT gen_random_uuid(), $1, 'new_follower', gen_random_uuid(), 'New follower', 'Someone followed you',
			'{}', 'follow-x', 'batched', $2
		FROM generate_series(1, $3::int)`, user.UUID(), at, n)
}

func (r *pushRig) wantRows(t *testing.T, want map[string]int) {
	t.Helper()
	var got map[string]int
	if err := r.pool.QueryRow(t.Context(), `SELECT coalesce(jsonb_object_agg(k, n), '{}')
		FROM (SELECT kind || '/' || state AS k, count(*) AS n FROM notifications GROUP BY 1) rows`).Scan(&got); err != nil ||
		!maps.Equal(got, want) {
		t.Fatalf("notification rows by kind and state = %v, %v, want %v", got, err, want)
	}
}

func followPush(to, from ids.UserID, who string) apns.Push {
	return apns.Push{
		UserID: to, Token: token('a'), Environment: apns.Sandbox, CollapseID: "follow-" + from.String(),
		Title: "New follower", Body: who + " followed you",
		Data: map[string]string{"kind": "new_follower", "user_id": from.String()},
	}
}

func digestPush(to ids.UserID, body string) apns.Push {
	return apns.Push{
		UserID: to, Token: token('a'), Environment: apns.Sandbox, CollapseID: "follow-digest",
		Title: "New followers", Body: body, Data: map[string]string{"kind": "follow_digest"},
	}
}

func digestSource(followee ids.UserID, date string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("notify.follow_digest:"+followee.String()+":"+date))
}

func TestNotify_NewFollower_CapThenDigest(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	followee := r.follower(t, "ada", "Ada")
	r.device(t, followee, token('a'))
	names := []string{"Bea", "Cyd", "Dan", "Eli", "Fay", "Gus"}
	followers := make([]ids.UserID, len(names))
	for i, name := range names {
		followers[i] = r.follower(t, strings.ToLower(name), name)
	}
	follows := make([]bus.Delivery, 0, 5)
	for _, follower := range followers[:5] {
		d, e := r.followed(t, followee, follower)
		wantVerdict(t, r.handleFollow(t, d, e), "", errs.VerdictAck)
		follows = append(follows, d)
	}

	singles := make([]apns.Push, 3)
	for i := range singles {
		singles[i] = followPush(followee, followers[i], names[i]+" (@"+strings.ToLower(names[i])+")")
	}
	if sent := r.sender.Sent(); !reflect.DeepEqual(sent, singles) {
		t.Fatalf("sent %+v after five follows, want only the first three: %+v", sent, singles)
	}
	for i, state := range []string{"delivered", "delivered", "delivered", "batched", "batched"} {
		r.wantStates(t, follows[i], map[ids.UserID]string{followee: state})
	}
	digest := r.followDigest()
	r.clock.Set(utc(2, 0, 59))
	wantTick(t, digest, 0, 0)
	if sent := r.sender.Sent(); len(sent) != 3 {
		t.Fatalf("sent %d pushes before 01:00 UTC, want the 3 singles only", len(sent))
	}

	r.clock.Set(utc(2, 1, 0))
	wantTick(t, digest, 1, 1)
	withDigest := slices.Concat(singles, []apns.Push{digestPush(followee, "2 more people followed you")})
	if sent := r.sender.Sent(); !reflect.DeepEqual(sent, withDigest) {
		t.Fatalf("sent %+v after the first tick past 01:00 UTC, want the digest once: %+v", sent, withDigest)
	}
	r.clock.Set(utc(2, 1, 30))
	wantTick(t, digest, 1, 0)
	if sent := r.sender.Sent(); len(sent) != 4 {
		t.Fatalf("sent %d pushes after a second tick, want still 4", len(sent))
	}

	r.clock.Set(utc(2, 2, 0))
	d, e := r.followed(t, followee, followers[5])
	wantVerdict(t, r.handleFollow(t, d, e), "", errs.VerdictAck)
	nextDay := followPush(followee, followers[5], "Gus (@gus)")
	if sent := r.sender.Sent(); len(sent) != 5 || !reflect.DeepEqual(sent[4], nextDay) {
		t.Fatalf("sent %+v, want the next day's first follow to push on its own: %+v", sent, nextDay)
	}
	r.wantStates(t, d, map[ids.UserID]string{followee: "delivered"})
	r.wantRows(t, map[string]int{"new_follower/delivered": 4, "new_follower/batched": 2, "follow_digest/delivered": 1})
	source := digestSource(followee, "2026-03-01")
	r.wantCount(t, "digest rows keyed by followee and day", 1, `SELECT count(*) FROM notifications
		WHERE kind = 'follow_digest' AND state = 'delivered' AND source_event_id = $1 AND user_id = $2`,
		source, followee.UUID())
	r.wantCount(t, "notification.sent events for the digest", 1, `SELECT count(*) FROM events
		WHERE type = 'notification.sent' AND payload->>'source_event_id' = $1`, source.String())
}

func TestNotify_NewFollower_DuplicateDeliveryKeepsCap(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	followee := r.follower(t, "ada", "Ada")
	r.device(t, followee, token('a'))
	follows, created := make([]bus.Delivery, 0, 4), make([]events.FollowCreated, 0, 4)
	for _, handle := range []string{"bea", "cyd", "dan", "eli"} {
		d, e := r.followed(t, followee, r.follower(t, handle, ""))
		wantVerdict(t, r.handleFollow(t, d, e), "", errs.VerdictAck)
		follows, created = append(follows, d), append(created, e)
	}
	want := map[string]int{"new_follower/delivered": 3, "new_follower/batched": 1}
	r.wantRows(t, want)

	for _, i := range []int{2, 3, 2} {
		wantVerdict(t, r.handleFollow(t, follows[i], created[i]), "", errs.VerdictAck)
	}

	r.wantRows(t, want)
	if sent := r.sender.Sent(); len(sent) != 3 {
		t.Fatalf("sent %d pushes after the redeliveries, want the 3 singles and no more", len(sent))
	}
	r.wantStates(t, follows[2], map[ids.UserID]string{followee: "delivered"})
	r.wantStates(t, follows[3], map[ids.UserID]string{followee: "batched"})
}

func TestNotify_NewFollower_ReachesOnlyTheFolloweeThroughTheModule(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	followee, follower := r.follower(t, "ada", "Ada"), r.follower(t, "bea", "Bea")
	r.device(t, followee, token('a'))
	r.device(t, follower, token('b'))
	deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.ids, Clock: r.clock}
	_, e := r.followed(t, followee, follower)

	d := r.dispatchTo(t, notify.New(deps, notify.WithSender(r.sender)), userActor(follower), e)

	want := followPush(followee, follower, "Bea (@bea)")
	if sent := r.sender.Sent(); !reflect.DeepEqual(sent, []apns.Push{want}) {
		t.Fatalf("sent %+v, want only %+v", sent, want)
	}
	r.wantStates(t, d, map[ids.UserID]string{followee: "delivered"})
	r.wantRecorded(t, d, 1)
}

func followerCard(id ids.UserID, name, handle string, deleted bool) map[ids.UserID]identity.UserCard {
	return map[ids.UserID]identity.UserCard{id: {ID: id, DisplayName: name, Handle: handle, Deleted: deleted}}
}

func TestNotify_NewFollower_NamesTheFollowerOrSomeone(t *testing.T) {
	t.Parallel()
	follower := ids.UserIDFrom(uuid.NewSHA1(uuid.NameSpaceOID, []byte("notify-follower")))
	e := events.FollowCreated{V: 1, FollowerID: follower.UUID()}
	for name, tc := range map[string]struct {
		cards map[ids.UserID]identity.UserCard
		want  string
	}{
		"a name and a handle":             {followerCard(follower, "Dana", "dana", false), "Dana (@dana) followed you"},
		"a handle that is the whole name": {followerCard(follower, "dana", "dana", false), "@dana followed you"},
		"a name and no handle":            {followerCard(follower, "Dana", "", false), "Dana followed you"},
		"no name and no handle":           {followerCard(follower, "", "", false), "Someone followed you"},
		"a deleted user that kept a name": {followerCard(follower, "Dana", "dana", true), "Someone followed you"},
		"a user the port does not know":   {nil, "Someone followed you"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			msg, err := app.NewFollower{Users: users{cards: tc.cards}}.Render(t.Context(), e, ids.UserID{})

			want := app.Message{
				Title: "New follower", Body: tc.want, CollapseID: "follow-" + follower.String(),
				Data: map[string]string{"kind": "new_follower", "user_id": follower.String()},
			}
			if err != nil || !reflect.DeepEqual(msg, want) {
				t.Fatalf("Render = %+v, %v, want %+v", msg, err, want)
			}
		})
	}
}

func TestNotify_NewFollower_ReturnsAUsersPortFailureAsIsAndNaks(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	followee, follower := r.follower(t, "ada", "Ada"), r.follower(t, "bea", "Bea")
	r.device(t, followee, token('a'))
	down := errs.New(errs.CodeDBUnavailable, "test.users")
	d, e := r.followed(t, followee, follower)

	err := handleKinds(t, r, r.sender, d, e, app.NewFollower{Users: users{err: down}})

	if !errors.Is(err, down) {
		t.Fatalf("Handle = %v, want the port error itself, %v", err, down)
	}
	wantVerdict(t, err, errs.CodeDBUnavailable, errs.VerdictNak)
	r.wantRows(t, map[string]int{})
	r.wantRecorded(t, d, 0)
}

func TestNotify_FollowDigest_SkipsDeletedFollowee(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	kept, gone := r.user(t, "active"), r.user(t, "deleted")
	r.device(t, kept, token('a'))
	r.device(t, gone, token('b'))
	r.batched(t, kept, utc(1, 12, 0), 2)
	r.batched(t, gone, utc(1, 12, 0), 2)
	r.clock.Set(utc(2, 1, 0))

	wantTick(t, r.followDigest(), 2, 1)

	if sent := r.sender.Sent(); !reflect.DeepEqual(sent, []apns.Push{digestPush(kept, "2 more people followed you")}) {
		t.Fatalf("sent %+v, want one digest, to the followee who is still there", sent)
	}
	r.wantCount(t, "digest rows of the deleted followee", 0,
		`SELECT count(*) FROM notifications WHERE kind = 'follow_digest' AND user_id = $1`, gone.UUID())
}

func TestNotify_FollowDigest_CountsTheUTCDayBeforeTheLagAndNamesEachDayOnce(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	followee := r.user(t, "active")
	r.device(t, followee, token('a'))
	for _, at := range []time.Time{
		utc(1, 0, 0).Add(-time.Second), utc(1, 0, 0), utc(2, 0, 0).Add(-time.Second), utc(2, 0, 0),
	} {
		r.batched(t, followee, at, 1)
	}
	digest := r.followDigest()

	r.clock.Set(utc(2, 1, 0))
	wantTick(t, digest, 1, 1)
	r.clock.Set(utc(3, 1, 0))
	wantTick(t, digest, 1, 1)

	want := []apns.Push{
		digestPush(followee, "2 more people followed you"), digestPush(followee, "1 more person followed you"),
	}
	if sent := r.sender.Sent(); !reflect.DeepEqual(sent, want) {
		t.Fatalf("sent %+v, want 1 March counted from 00:00:00 to 23:59:59 and 2 March on its own: %+v", sent, want)
	}
	for _, date := range []string{"2026-03-01", "2026-03-02"} {
		r.wantCount(t, "digest rows for "+date, 1, `SELECT count(*) FROM notifications
			WHERE kind = 'follow_digest' AND source_event_id = $1`, digestSource(followee, date))
	}
}

func TestNotify_FollowDigest_ASendFailureLeavesTheRowPendingAndTheNextTickSendsIt(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	followee := r.user(t, "active")
	r.device(t, followee, token('a'))
	r.batched(t, followee, utc(1, 12, 0), 4)
	r.sender.Reply(token('a'), apns.Result{Status: 429})
	digest := r.followDigest()
	r.clock.Set(utc(2, 1, 0))

	got, err := tick(t, digest)

	wantVerdict(t, err, errs.CodeAPNSUnavailable, errs.VerdictNak)
	if got.Scanned != 1 || got.Changed != 1 {
		t.Fatalf("Tick = %+v, want scanned 1 and changed 1", got)
	}
	r.wantRows(t, map[string]int{"new_follower/batched": 4, "follow_digest/pending": 1})

	r.sender.Reply(token('a'), apns.Result{Status: 200})
	r.clock.Set(utc(2, 2, 0))
	wantTick(t, digest, 1, 0)
	wantTick(t, digest, 1, 0)

	if sent := r.sender.Sent(); len(sent) != 2 || sent[1].Body != "4 more people followed you" {
		t.Fatalf("sent %+v, want the same digest retried once and then left alone", sent)
	}
	r.wantRows(t, map[string]int{"new_follower/batched": 4, "follow_digest/delivered": 1})
}

func TestNotify_FollowDigest_FailuresReturnAndADigestThatFailsDoesNotHoldBackTheRest(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeDBUnavailable, "test.users")
	refuseFourMore := []string{
		`CREATE FUNCTION refuse() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
			IF NEW.body LIKE '%4 more%' THEN RAISE EXCEPTION 'refused'; END IF; RETURN NEW; END $$`,
		`CREATE TRIGGER refuse BEFORE INSERT ON notifications FOR EACH ROW EXECUTE FUNCTION refuse()`,
	}
	for name, tc := range map[string]struct {
		opts    []notify.Option
		arrange []string
		scanned int
		changed int
		want    errs.Code
	}{
		"counts":     {nil, []string{`ALTER TABLE notifications RENAME TO gone`}, 0, 0, errs.CodeDBUnavailable},
		"users port": {[]notify.Option{notify.WithUsers(users{err: down})}, nil, 2, 0, errs.CodeDBUnavailable},
		"one digest": {nil, refuseFourMore, 2, 1, errs.CodeInternal},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newPushRig(t)
			first, second := r.user(t, "active"), r.user(t, "active")
			r.device(t, first, token('a'))
			r.device(t, second, token('b'))
			r.batched(t, first, utc(1, 12, 0), 2)
			r.batched(t, second, utc(1, 12, 0), 4)
			r.clock.Set(utc(2, 1, 0))
			for _, sql := range tc.arrange {
				r.exec(t, sql)
			}

			got, err := tick(t, r.followDigest(tc.opts...))

			if errs.CodeOf(err) != tc.want || got.Scanned != tc.scanned || got.Changed != tc.changed {
				t.Fatalf("Tick = %+v, %v, want scanned %d, changed %d and %s",
					got, err, tc.scanned, tc.changed, tc.want)
			}
		})
	}
}

type chunked struct {
	kept  uuid.UUID
	sizes []int
}

func (c *chunked) UsersByID(_ context.Context, userIDs []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	c.sizes = append(c.sizes, len(userIDs))
	if len(userIDs) > identity.MaxUsersByID {
		return nil, errs.New(errs.CodeInvalidInput, "test.users")
	}
	cards := make(map[ids.UserID]identity.UserCard, len(userIDs))
	for _, id := range userIDs {
		cards[id] = identity.UserCard{ID: id, Deleted: id.UUID() != c.kept}
	}
	return cards, nil
}

func TestNotify_FollowDigest_AsksTheUsersPortInChunksItAccepts(t *testing.T) {
	t.Parallel()
	r := newPushRig(t)
	const followees = identity.MaxUsersByID + 1
	r.exec(t, `INSERT INTO users (id, privy_user_id, login_provider, auth_state, auth_state_changed_at,
		account_status, created_at, updated_at)
		SELECT gen_random_uuid(), 'did:privy:digest-' || g, 'sms', 'CREATED', now(), 'active', now(), now()
		FROM generate_series(1, $1::int) g`, followees)
	r.exec(t, `INSERT INTO notifications
		(id, user_id, kind, source_event_id, title, body, data, collapse_id, state, created_at)
		SELECT gen_random_uuid(), u.id, 'new_follower', gen_random_uuid(), 't', 'b', '{}', 'c', 'batched', $1
		FROM users u`, utc(1, 12, 0))
	last := &chunked{}
	if err := r.pool.QueryRow(t.Context(), `SELECT user_id FROM notifications ORDER BY user_id DESC LIMIT 1`).
		Scan(&last.kept); err != nil {
		t.Fatal(err)
	}
	r.device(t, ids.UserIDFrom(last.kept), token('a'))
	r.clock.Set(utc(2, 1, 0))

	wantTick(t, r.followDigest(notify.WithUsers(last)), followees, 1)

	if want := []int{identity.MaxUsersByID, 1}; !slices.Equal(last.sizes, want) {
		t.Fatalf("UsersByID asked for %v users at a time, want %v", last.sizes, want)
	}
	if sent := r.sender.Sent(); len(sent) != 1 || sent[0].UserID != ids.UserIDFrom(last.kept) {
		t.Fatalf("sent %+v, want one digest, to the followee in the second chunk", sent)
	}
}
