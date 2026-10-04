package identity_test

import (
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
)

func TestSearchUsers_HandlePrefixFirst(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	caller := f.seed(t, portSeed{handle: "caller"})
	maya := f.seed(t, portSeed{handle: "maya"})
	f.seed(t, portSeed{handle: "kai", name: "Mark"})
	for i := range 25 {
		f.seed(t, portSeed{
			handle: "other" + strings.Repeat("a", i%10) + string(rune('a'+i%26)), name: "Ma result",
		})
	}
	users, err := app.SearchUsers(t.Context(), f.pool, caller.ID, "ma")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 20 {
		t.Fatalf("returned %d users, want 20", len(users))
	}
	if users[0].ID != maya.ID {
		t.Fatalf("first user = %s, want %s", users[0].ID, maya.ID)
	}
}

func TestSearchUsers_Excludes(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	caller := f.seed(t, portSeed{handle: "caller"})
	f.seed(t, portSeed{handle: "banned", name: "Maya", status: "banned"})
	f.seed(t, portSeed{handle: "deleted", name: "Maya", status: "deleted"})
	f.seed(t, portSeed{name: "Maya"})
	visible := f.seed(t, portSeed{handle: "visible", name: "Maya"})
	users, err := app.SearchUsers(t.Context(), f.pool, caller.ID, "maya")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].ID != visible.ID {
		t.Fatalf("users = %#v, want only %s", users, visible.ID)
	}
}

func TestSearchUsers_AtAndCase(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	caller := f.seed(t, portSeed{handle: "caller"})
	maya := f.seed(t, portSeed{handle: "maya"})
	percent := f.seed(t, portSeed{handle: "pct_user", name: "100%match"})
	underscore := f.seed(t, portSeed{handle: "under_user", name: "under_score"})
	upper, err := app.SearchUsers(t.Context(), f.pool, caller.ID, "@MAYA")
	if err != nil {
		t.Fatal(err)
	}
	lower, err := app.SearchUsers(t.Context(), f.pool, caller.ID, "maya")
	if err != nil {
		t.Fatal(err)
	}
	if len(upper) != len(lower) || len(upper) != 1 || upper[0].ID != maya.ID {
		t.Fatalf("case-insensitive users = %#v and %#v", upper, lower)
	}
	for _, tc := range []struct {
		query string
		want  string
	}{{"%m", percent.ID.String()}, {"_s", underscore.ID.String()}} {
		users, err := app.SearchUsers(t.Context(), f.pool, caller.ID, tc.query)
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || users[0].ID.String() != tc.want {
			t.Fatalf("%q users = %#v, want %s", tc.query, users, tc.want)
		}
	}
}

func TestSearchUsers_TooShort(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	for _, query := range []string{"a", "@a", strings.Repeat("a", 51)} {
		_, err := app.SearchUsers(t.Context(), f.pool, f.newID(t), query)
		wantCode(t, err, errs.CodeInvalidInput)
	}
}

func TestSearchUsers_UsesIndexes(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO users
  (id, privy_user_id, handle, login_provider, auth_state_changed_at, created_at, updated_at)
SELECT lpad(to_hex(i), 32, '0')::uuid, 'search-index-' || i, 'ma' || lpad(i::text, 8, '0'),
  'sms', $1, $1, $1
FROM generate_series(1, 10000) AS i`, f.now)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := f.pool.Query(t.Context(), `EXPLAIN SELECT id FROM users
WHERE handle LIKE 'ma%' ESCAPE '!'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	lines := []string{}
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	plan := strings.Join(lines, "\n")
	if !strings.Contains(plan, "users_handle_pattern_idx") {
		t.Fatalf("EXPLAIN plan = %q", plan)
	}
}
