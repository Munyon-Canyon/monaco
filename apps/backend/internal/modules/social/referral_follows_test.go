package social_test

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type followRow struct{ Follower, Followee, Source string }

func (f fixture) referralFollows() adapters.ReferralFollows {
	return adapters.ReferralFollows{Users: f.users, IDs: f.gen}
}

func (f fixture) attributed(referrer, referee ids.UserID) events.ReferralAttributed {
	return events.ReferralAttributed{
		V: 1, ReferralID: f.gen.NewV7(), Referrer: referrer.UUID(), Referee: referee.UUID(),
		CodeKind: "random", Source: "clipboard", AttributedAt: f.now,
	}
}

func (f fixture) deliver(t *testing.T, e events.ReferralAttributed) error {
	t.Helper()
	return f.deliverIn(t.Context(), e)
}

func (f fixture) deliverIn(ctx context.Context, e events.ReferralAttributed) error {
	ctx = observability.WithActor(ctx, "system:social.referral_follows")
	h := f.referralFollows()
	followable, err := h.Fetch(ctx, e)
	if err != nil {
		return err
	}
	return db.New(f.pool, f.gen, f.clock).Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return h.Apply(ctx, tx, e, followable, f.now)
	})
}

func (f fixture) followRows(ctx context.Context, t *testing.T) []followRow {
	t.Helper()
	rows, err := f.pool.Query(ctx,
		`SELECT follower_id::text, followee_id::text, source FROM follows WHERE deleted_at IS NULL
		 ORDER BY follower_id, followee_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []followRow
	for rows.Next() {
		var r followRow
		if err := rows.Scan(&r.Follower, &r.Followee, &r.Source); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (f fixture) createdEvents(ctx context.Context, t *testing.T) []events.FollowCreated {
	t.Helper()
	rows, err := f.pool.Query(ctx, `SELECT payload FROM events WHERE type = 'follow.created' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []events.FollowCreated
	for rows.Next() {
		var payload []byte
		var e events.FollowCreated
		if err := rows.Scan(&payload); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(payload, &e); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func wantRows(source string, pairs ...[2]ids.UserID) []followRow {
	out := make([]followRow, len(pairs))
	for i, p := range pairs {
		out[i] = followRow{Follower: p[0].String(), Followee: p[1].String(), Source: source}
	}
	return sortedRows(out)
}

func sortedRows(rows []followRow) []followRow {
	slices.SortFunc(rows, func(a, b followRow) int {
		return cmp.Or(cmp.Compare(a.Follower, b.Follower), cmp.Compare(a.Followee, b.Followee))
	})
	return rows
}

func TestReferralFollows_isRegisteredOnItsOwnDurable(t *testing.T) {
	t.Parallel()
	for _, consumer := range social.New(module.Deps{}).Consumers() {
		if consumer.Durable == "social_referral_follows" {
			if len(consumer.Handlers) != 1 || consumer.Handlers[0].Name != "social.referral_follows" {
				t.Fatalf("handlers = %+v, want only social.referral_follows", consumer.Handlers)
			}
			return
		}
	}
	t.Fatal("social_referral_follows is not registered")
}

func TestReferralFollows_BothDirections(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.deliver(t, f.attributed(f.alice, f.bob)); err != nil {
		t.Fatal(err)
	}
	want := wantRows("referral", [2]ids.UserID{f.alice, f.bob}, [2]ids.UserID{f.bob, f.alice})
	if got := f.followRows(t.Context(), t); !slices.Equal(got, want) {
		t.Fatalf("follows = %+v, want %+v", got, want)
	}
	created := f.createdEvents(t.Context(), t)
	if len(created) != 2 {
		t.Fatalf("follow.created events = %+v, want 2", created)
	}
	if created[0].FollowerID != f.bob.UUID() || created[0].FolloweeID != f.alice.UUID() ||
		created[1].FollowerID != f.alice.UUID() || created[1].FolloweeID != f.bob.UUID() {
		t.Fatalf("events = %+v, want the referee following first, then the referrer", created)
	}
	for _, e := range created {
		if e.Source != "referral" || e.V != 1 || !e.CreatedAt.Equal(f.now) {
			t.Fatalf("event = %+v, want source referral at %s", e, f.now)
		}
	}
}

func TestReferralFollows_OneDirectionExists(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.follows(t, domain.SourceFeed); err != nil {
		t.Fatal(err)
	}
	if err := f.deliver(t, f.attributed(f.alice, f.bob)); err != nil {
		t.Fatal(err)
	}
	got := f.followRows(t.Context(), t)
	want := []followRow{
		{Follower: f.alice.String(), Followee: f.bob.String(), Source: "feed"},
		{Follower: f.bob.String(), Followee: f.alice.String(), Source: "referral"},
	}
	if want = sortedRows(want); !slices.Equal(got, want) {
		t.Fatalf("follows = %+v, want %+v", got, want)
	}
	referral := 0
	for _, e := range f.createdEvents(t.Context(), t) {
		if e.Source == "referral" {
			referral++
			if e.FollowerID != f.bob.UUID() || e.FolloweeID != f.alice.UUID() {
				t.Fatalf("referral event = %+v, want the missing bob -> alice direction", e)
			}
		}
	}
	if total := len(f.createdEvents(t.Context(), t)); referral != 1 || total != 2 {
		t.Fatalf(
			"referral events %d of %d, want 1 of 2 (the feed follow's and the missing direction's)",
			referral,
			total,
		)
	}
}

func TestReferralFollows_BannedReferrer(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.deliver(t, f.attributed(f.banned, f.alice)); err != nil {
		t.Fatalf("a banned referrer must ack, got %v", err)
	}
	if rows, created := f.followRows(t.Context(), t), f.createdEvents(t.Context(), t); len(rows) != 0 ||
		len(created) != 0 {
		t.Fatalf("follows %+v, follow.created %+v; want nothing", rows, created)
	}
}

func TestReferralFollows_skipsBothDirectionsWhenEitherUserCannotBeFollowed(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cases := map[string][2]ids.UserID{
		"banned referee":   {f.alice, f.banned},
		"deleted referrer": {f.deleted, f.alice},
		"deleted referee":  {f.alice, f.deleted},
		"unknown referee":  {f.alice, f.stranger},
		"self referral":    {f.alice, f.alice},
	}
	for name, pair := range cases {
		if err := f.deliver(t, f.attributed(pair[0], pair[1])); err != nil {
			t.Fatalf("%s: must ack, got %v", name, err)
		}
		if rows, created := f.followRows(t.Context(), t), f.createdEvents(t.Context(), t); len(rows) != 0 ||
			len(created) != 0 {
			t.Fatalf("%s: follows %+v, follow.created %+v; want nothing", name, rows, created)
		}
	}
}

func TestReferralFollows_aSuspendedUserStillGetsTheFollows(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if err := f.deliver(t, f.attributed(f.bob, f.alice)); err != nil {
		t.Fatal(err)
	}
	if rows := f.followRows(t.Context(), t); len(rows) != 2 {
		t.Fatalf("follows = %+v, want both directions", rows)
	}
}

func TestReferralFollows_Redelivery(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	e := f.attributed(f.alice, f.bob)
	for range 2 {
		if err := f.deliver(t, e); err != nil {
			t.Fatal(err)
		}
	}
	want := wantRows("referral", [2]ids.UserID{f.alice, f.bob}, [2]ids.UserID{f.bob, f.alice})
	if got := f.followRows(t.Context(), t); !slices.Equal(got, want) {
		t.Fatalf("follows = %+v, want %+v", got, want)
	}
	if created := f.createdEvents(t.Context(), t); len(created) != 2 {
		t.Fatalf("follow.created = %+v, want 2 after two deliveries", created)
	}
}

func TestReferralFollows_anIdentityFailureIsReturnedForARetry(t *testing.T) {
	t.Parallel()
	down := errs.New(errs.CodeUpstreamUnavailable, "test")
	for name, queued := range map[string][]error{
		"first lookup":  {down},
		"second lookup": {nil, down},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			for _, err := range queued {
				f.users.FailOnce("UsersByID", err)
			}
			wantCode(t, f.deliver(t, f.attributed(f.alice, f.bob)), errs.CodeUpstreamUnavailable)
			if rows := f.followRows(t.Context(), t); len(rows) != 0 {
				t.Fatalf("follows = %+v, want none", rows)
			}
		})
	}
}

func TestReferralFollows_aDatabaseFailureIsInternal(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE follows`); err != nil {
		t.Fatal(err)
	}
	wantCode(t, f.deliver(t, f.attributed(f.alice, f.bob)), errs.CodeInternal)
}

func TestReferralFollows_aTransientDatabaseFailureNaksInsteadOfTerming(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	e := f.attributed(f.alice, f.bob)
	ctx, cancel := context.WithCancel(observability.WithActor(t.Context(), "system:social.referral_follows"))
	defer cancel()
	h := f.referralFollows()
	err := db.New(f.pool, f.gen, f.clock).Do(ctx, func(ctx context.Context, tx db.Tx) error {
		cancel()
		return h.Apply(ctx, tx, e, true, f.now)
	})
	wantCode(t, err, errs.CodeDBUnavailable)
	if errs.VerdictFor(errs.CodeOf(err)) != errs.VerdictNak {
		t.Fatalf("verdict for %v = term, want nak", err)
	}
	if got := f.followRows(t.Context(), t); len(got) != 0 {
		t.Fatalf("follows = %+v after a rolled back delivery, want none", got)
	}
}
