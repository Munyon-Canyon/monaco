package agents

import (
	"context"
	"crypto/rand"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func TestTakeTestDB_failsWhenNoSlotCanBeTaken(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		setup func(t *testing.T, slots string)
		want  string
	}{
		"slot dir is a file": {
			setup: func(t *testing.T, slots string) { t.Helper(); writeFile(t, slots, "") },
			want:  "take a test database slot",
		},
		"first slot unopenable": {
			setup: func(t *testing.T, slots string) { t.Helper(); mkdir(t, filepath.Join(slots, "0")) },
			want:  "take a test database slot",
		},
		"a later slot unopenable": {
			setup: func(t *testing.T, slots string) { t.Helper(); mkdir(t, filepath.Join(slots, "5")) },
			want:  "count test database slots",
		},
		"every slot held": {
			setup: func(t *testing.T, slots string) {
				t.Helper()
				mkdir(t, slots)
				for slot := range testDBSlots {
					held, ok, err := tryLock(filepath.Join(slots, strconv.Itoa(slot)))
					if err != nil || !ok {
						t.Fatalf("hold slot %d: %v %v", slot, ok, err)
					}
					t.Cleanup(func() { _ = held.Close() })
				}
			},
			want: "every test database slot is held: 16 in ",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := &Env{Common: t.TempDir(), Work: t.TempDir()}
			c.setup(t, filepath.Join(env.Common, ".monaco", "test-db"))
			if _, _, err := env.takeTestDB(); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("takeTestDB() = %v, want %q", err, c.want)
			}
		})
	}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
}

func TestCheck_failsTheDatabaseRowBeforeItsCommandsWhenNoTestDatabaseSlotCanBeTaken(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.affected = "./internal/a\n"
	writeFile(t, filepath.Join(h.Env(t).Common, ".monaco", "test-db"), "")
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "take a test database slot") ||
		slices.ContainsFunc(
			h.calls,
			func(c string) bool { return strings.Contains(c, "docker") || strings.Contains(c, "go test ") },
		) ||
		ticketCount(t, h.Env(t), "db") != 0 {
		t.Fatalf("an unusable slot dir: %d %q %v", code, stderr, h.calls)
	}
}

func TestDropDeadRunClones_dropsOnlyTheClonesOfRunsWithNoConnection(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	admin := connect(t, config.TestDBURL(os.Environ()))
	run := func() string { return "t_" + strings.ToLower(rand.Text()[:8]) + "_" }
	live, dead := run(), run()
	template := "testdb_tpl_" + strings.ToLower(rand.Text()[:8])
	names := []string{live + "held_a", live + "idle_b", dead + "one_c", dead + "two_d", template}
	for _, name := range names {
		if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			drop := "DROP DATABASE IF EXISTS " + pgx.Identifier{name}.Sanitize() + " WITH (FORCE)"
			_, _ = admin.Exec(context.Background(), drop)
		})
	}
	held, err := url.Parse(config.TestDBURL(os.Environ()))
	if err != nil {
		t.Fatal(err)
	}
	held.Path = "/" + live + "held_a"
	connect(t, held.String())

	rows, err := admin.Query(ctx, dropDeadRunClones)
	if err != nil {
		t.Fatal(err)
	}
	drops, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	for _, drop := range drops {
		if strings.Contains(drop, live) || strings.Contains(drop, template) {
			t.Fatalf("%q drops a live run's clone or a template", drop)
		}
		if strings.Contains(drop, dead) {
			if _, err := admin.Exec(ctx, drop); err != nil {
				t.Fatal(err)
			}
		}
	}
	rows, err = admin.Query(ctx, `SELECT datname FROM pg_database WHERE datname = ANY($1) ORDER BY datname`, names)
	if err != nil {
		t.Fatal(err)
	}
	left, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	want := []string{live + "held_a", live + "idle_b", template}
	slices.Sort(want)
	if !slices.Equal(left, want) {
		t.Fatalf("after the drop %v remain, want %v (statements %v)", left, want, drops)
	}
}

func connect(t *testing.T, url string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

func TestTestEnv_runsTenChaosSeedsUnlessTheEnvironmentSaysOtherwise(t *testing.T) {
	t.Setenv("CHAOS_SEEDS", "")
	got := testDB{slot: 2}.testEnv()
	want := []string{"env", "TEST_DATABASE_URL=" + testDB{slot: 2}.url(), "CHAOS_SEEDS=10"}
	if !slices.Equal(got, want) {
		t.Fatalf("testEnv = %q, want %q", got, want)
	}
	t.Setenv("CHAOS_SEEDS", "50")
	got = testDB{}.testEnv()
	if slices.ContainsFunc(got, func(s string) bool { return strings.HasPrefix(s, "CHAOS_SEEDS=") }) {
		t.Fatalf("testEnv = %q, want the caller's CHAOS_SEEDS left alone", got)
	}
}
