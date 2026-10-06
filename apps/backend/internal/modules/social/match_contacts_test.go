package social_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type contactFixture struct {
	fixture
	readers  app.Users
	match    *app.MatchContactsHandler
	followOn *app.FollowHandler
	unfollow *app.UnfollowHandler
}

func newContactFixture(t *testing.T) contactFixture {
	t.Helper()
	base := newFixture(t)
	readers := identity.New(module.Deps{Pool: base.pool}).Queries()
	uow := db.New(base.pool, base.gen, base.clock)
	return contactFixture{
		fixture:  base,
		readers:  readers,
		match:    app.NewMatchContactsHandler(app.MatchContactsDeps{UoW: uow, Users: readers, Clock: base.clock}),
		followOn: app.NewFollowHandler(app.FollowDeps{UoW: uow, Users: readers, IDs: base.gen, Clock: base.clock}),
		unfollow: app.NewUnfollowHandler(uow, base.clock),
	}
}

func (f contactFixture) ctx(t *testing.T, logs *testkit.Logs) context.Context {
	t.Helper()
	ctx := observability.WithActor(t.Context(), "user:"+f.alice.String())
	if logs == nil {
		return ctx
	}
	logger := observability.NewLogger(config.Config{Env: config.EnvTest}, logs)
	return observability.WithLogger(ctx, logger)
}

func contactHash(e164 string) string {
	sum := sha256.Sum256([]byte(e164))
	return hex.EncodeToString(sum[:])
}

func setPhone(t *testing.T, pool *pgxpool.Pool, id ids.UserID, e164 string, verified bool) {
	t.Helper()
	sum := sha256.Sum256([]byte(e164))
	var verifiedAt any
	if verified {
		verifiedAt = clock.Real{}.Now().UTC()
	}
	_, err := pool.Exec(t.Context(), `UPDATE users SET phone_e164 = $2, phone_hash = $3, phone_verified_at = $4
		WHERE id = $1`, id.UUID(), e164, sum[:], verifiedAt)
	if err != nil {
		t.Fatal(err)
	}
}

func seedPhoneUser(t *testing.T, pool *pgxpool.Pool, handle, e164, status string, verified bool) ids.UserID {
	t.Helper()
	user := testkit.SeedUser(t, pool, testkit.UserOpts{Handle: handle, AccountStatus: status})
	setPhone(t, pool, user.ID, e164, verified)
	return user.ID
}

func (f contactFixture) matchRows(t *testing.T, user ids.UserID) []uuid.UUID {
	t.Helper()
	rows, err := f.pool.Query(t.Context(),
		`SELECT matched_user_id FROM contact_matches WHERE user_id = $1 ORDER BY matched_user_id`, user.UUID())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		got = append(got, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return got
}

func assertHashesAbsent(t *testing.T, pool *pgxpool.Pool, hashes []string) {
	t.Helper()
	body := publicRowsText(t, pool)
	for _, hash := range hashes {
		if strings.Contains(body, hash) {
			t.Fatalf("unmatched hash %s was stored", hash)
		}
	}
}

func publicRowsText(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var dump strings.Builder
	for _, name := range publicTableNames(t, pool) {
		writeTableJSON(t, pool, name, &dump)
	}
	return dump.String()
}

func publicTableNames(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	tables, err := pool.Query(t.Context(), `SELECT tablename FROM pg_tables WHERE schemaname = 'public'`)
	if err != nil {
		t.Fatal(err)
	}
	defer tables.Close()
	var names []string
	for tables.Next() {
		var name string
		if err := tables.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := tables.Err(); err != nil {
		t.Fatal(err)
	}
	return names
}

func writeTableJSON(t *testing.T, pool *pgxpool.Pool, name string, dump *strings.Builder) {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT row_to_json(t)::text FROM `+quoteIdent(name)+` t`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		dump.WriteString(line)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}

func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func threeVerifiedAmong(
	t *testing.T, f contactFixture, n int,
) (hashes []string, want map[uuid.UUID]struct{}, unmatched []string) {
	t.Helper()
	hashes = make([]string, n)
	want = map[uuid.UUID]struct{}{}
	for i := range hashes {
		e164 := fmt.Sprintf("+1415555%04d", i)
		hashes[i] = contactHash(e164)
		if i >= 3 {
			unmatched = append(unmatched, hashes[i])
			continue
		}
		id := seedPhoneUser(t, f.pool, fmt.Sprintf("pal_%d", i), e164, "active", true)
		want[id.UUID()] = struct{}{}
	}
	return hashes, want, unmatched
}

func assertStoredMatches(t *testing.T, f contactFixture, user ids.UserID, want map[uuid.UUID]struct{}) {
	t.Helper()
	got := f.matchRows(t, user)
	if len(got) != len(want) {
		t.Fatalf("contact_matches = %d, want %d", len(got), len(want))
	}
	for _, id := range got {
		if _, ok := want[id]; !ok {
			t.Fatalf("stored %s, which was not one of the verified users", id)
		}
	}
}

func assertContactsMatchedLog(t *testing.T, logged string, hashes []string) {
	t.Helper()
	if !strings.Contains(logged, "social.contacts_matched") || !strings.Contains(logged, `"submitted":500`) ||
		!strings.Contains(logged, `"matched":3`) {
		t.Fatalf("log = %s, want submitted 500 and matched 3", logged)
	}
	for _, hash := range hashes {
		if strings.Contains(logged, hash) {
			t.Fatalf("log contains hash %s", hash)
		}
	}
}

func assertDismissedRowSurvivesRematch(
	t *testing.T, f contactFixture, me ids.UserID, hashes []string, want map[uuid.UUID]struct{},
) {
	t.Helper()
	var dismissed uuid.UUID
	for id := range want {
		dismissed = id
		break
	}
	if _, err := f.pool.Exec(t.Context(), `UPDATE contact_matches SET dismissed_at = $3
		WHERE user_id = $1 AND matched_user_id = $2`, me.UUID(), dismissed, f.now); err != nil {
		t.Fatal(err)
	}
	if err := f.match.Handle(f.ctx(t, nil), app.MatchContacts{User: me, Hashes: hashes}); err != nil {
		t.Fatal(err)
	}
	var dismissedAt *time.Time
	var created time.Time
	err := f.pool.QueryRow(t.Context(), `SELECT created_at, dismissed_at FROM contact_matches
		WHERE user_id = $1 AND matched_user_id = $2`, me.UUID(), dismissed).Scan(&created, &dismissedAt)
	if err != nil {
		t.Fatal(err)
	}
	if dismissedAt == nil || !created.Equal(f.now) || len(f.matchRows(t, me)) != 3 {
		t.Fatalf("rematch created %s dismissed %v rows %d, want the original row still dismissed",
			created, dismissedAt, len(f.matchRows(t, me)))
	}
}

func TestMatchContacts_StoresOnlyMatches(t *testing.T) {
	t.Parallel()
	f := newContactFixture(t)
	me := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "matcher"})
	hashes, want, unmatched := threeVerifiedAmong(t, f, 500)
	var logs testkit.Logs
	if err := f.match.Handle(f.ctx(t, &logs), app.MatchContacts{User: me.ID, Hashes: hashes}); err != nil {
		t.Fatal(err)
	}
	assertStoredMatches(t, f, me.ID, want)
	assertHashesAbsent(t, f.pool, unmatched)
	assertContactsMatchedLog(t, string(logs.Bytes()), hashes)
	assertDismissedRowSurvivesRematch(t, f, me.ID, hashes, want)
}

func TestMatchContacts_ExcludesSelfUnverifiedBannedDeleted(t *testing.T) {
	t.Parallel()
	f := newContactFixture(t)
	me := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "self_user"})
	setPhone(t, f.pool, me.ID, "+14155551000", true)
	good := seedPhoneUser(t, f.pool, "good_one", "+14155551001", "active", true)
	seedPhoneUser(t, f.pool, "no_verify", "+14155551002", "active", false)
	seedPhoneUser(t, f.pool, "banned_u", "+14155551003", "banned", true)
	seedPhoneUser(t, f.pool, "deleted_u", "+14155551004", "deleted", true)
	hashes := []string{
		contactHash("+14155551000"), contactHash("+14155551001"), contactHash("+14155551002"),
		contactHash("+14155551003"), contactHash("+14155551004"),
	}
	if err := f.match.Handle(f.ctx(t, nil), app.MatchContacts{User: me.ID, Hashes: hashes}); err != nil {
		t.Fatal(err)
	}
	got := f.matchRows(t, me.ID)
	if len(got) != 1 || got[0] != good.UUID() {
		t.Fatalf("matches = %v, want only %s", got, good)
	}
}

func TestMatchContacts_rejectsInvalidHashes(t *testing.T) {
	t.Parallel()
	f := newContactFixture(t)
	me := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "rejector"})
	valid := contactHash("+14155550123")
	tooMany := make([]string, domain.MaxContactHashes+1)
	for i := range tooMany {
		tooMany[i] = valid
	}
	tests := []struct {
		name   string
		hashes []string
		want   errs.Code
	}{
		{"empty", nil, errs.CodeContactHashesInvalid},
		{"uppercase", []string{strings.ToUpper(valid)}, errs.CodeContactHashesInvalid},
		{"short", []string{valid[:63]}, errs.CodeContactHashesInvalid},
		{"too many", tooMany, errs.CodeTooManyContactHashes},
	}
	for _, tt := range tests {
		err := f.match.Handle(f.ctx(t, nil), app.MatchContacts{User: me.ID, Hashes: tt.hashes})
		if got := errs.CodeOf(err); err == nil || got != tt.want {
			t.Errorf("%s: err = %v (code %s), want %s", tt.name, err, got, tt.want)
		}
	}
	if rows := f.matchRows(t, me.ID); len(rows) != 0 {
		t.Fatalf("rows = %d, want 0", len(rows))
	}
}

func TestMatchContacts_anUnknownHashStoresNothing(t *testing.T) {
	t.Parallel()
	f := newContactFixture(t)
	me := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "lonely"})
	var logs testkit.Logs
	err := f.match.Handle(f.ctx(t, &logs), app.MatchContacts{
		User: me.ID, Hashes: []string{contactHash("+19995550100")},
	})
	if err != nil || len(f.matchRows(t, me.ID)) != 0 || !strings.Contains(string(logs.Bytes()), `"matched":0`) {
		t.Fatalf("err = %v rows = %v log = %s", err, f.matchRows(t, me.ID), logs.Bytes())
	}
}

func TestMatchContacts_returnsTheDirectoryAndInsertErrors(t *testing.T) {
	t.Parallel()
	f := newContactFixture(t)
	me := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "broken"})
	hash := contactHash("+14155550199")
	err := app.NewMatchContactsHandler(app.MatchContactsDeps{
		UoW: db.New(f.pool, f.gen, f.clock), Users: brokenDirectory{}, Clock: f.clock,
	}).Handle(f.ctx(t, nil), app.MatchContacts{User: me.ID, Hashes: []string{hash}})
	wantCode(t, err, errs.CodeInternal)
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE contact_matches`); err != nil {
		t.Fatal(err)
	}
	seedPhoneUser(t, f.pool, "still_ok", "+14155550199", "active", true)
	err = f.match.Handle(f.ctx(t, nil), app.MatchContacts{User: me.ID, Hashes: []string{hash}})
	wantCode(t, err, errs.CodeInternal)
}

type brokenDirectory struct{}

func (brokenDirectory) UsersByID(context.Context, []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	return nil, errs.New(errs.CodeInternal, "test.users")
}

func (brokenDirectory) UsersByPhoneHashes(context.Context, [][]byte) (map[string]ids.UserID, error) {
	return nil, errs.New(errs.CodeInternal, "test.phones")
}

func (brokenDirectory) UserIDsByHandles(context.Context, []string) (map[string]ids.UserID, error) {
	return nil, errs.New(errs.CodeInternal, "test.handles")
}
