package testkit_test

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func insertEvents(t *testing.T, db *pgxpool.Pool, g *testkit.IDs, n int, actor string) {
	t.Helper()
	for range n {
		id := ids.New[struct{}](g)
		_, err := db.Exec(context.Background(), `
			INSERT INTO events (id, aggregate_type, aggregate_id, type, payload, actor_type, actor_id)
			VALUES ($1, 'system', $1, 'system.ping', '{"v":1}', 'system', $2)`, id, actor)
		if err != nil {
			t.Fatal(err)
		}
	}
}

func countEvents(t *testing.T, db *pgxpool.Pool, actor string) (total, foreign int) {
	t.Helper()
	err := db.QueryRow(context.Background(),
		`SELECT count(*), count(*) FILTER (WHERE actor_id <> $1) FROM events`, actor).Scan(&total, &foreign)
	if err != nil {
		t.Fatal(err)
	}
	return total, foreign
}

func waitFor(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(30 * time.Second):
		t.Fatalf("waited 30s for %s; the isolation test needs -parallel 2 or more", what)
	}
}

func TestDBIsolatesParallelTestsWritingTheSameTable(t *testing.T) {
	t.Parallel()
	wrote := map[string]chan struct{}{"a": make(chan struct{}), "b": make(chan struct{})}
	other := map[string]string{"a": "b", "b": "a"}
	for i, name := range []string{"a", "b"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			db := testkit.DB(t)
			insertEvents(t, db, testkit.NewIDs(1), 3+i, name)
			close(wrote[name])
			waitFor(t, wrote[other[name]], "the other writer")
			total, foreign := countEvents(t, db, name)
			if total != 3+i || foreign != 0 {
				t.Fatalf(
					"%s sees %d events, %d of them from the other test; want only its own %d",
					name,
					total,
					foreign,
					3+i,
				)
			}
		})
	}
}

func TestDBDuplicateTestNamePanics(t *testing.T) {
	t.Parallel()
	testkit.DB(t)
	defer func() {
		msg := fmt.Sprint(recover())
		if !strings.Contains(msg, t.Name()+" already has a database in this run") {
			t.Fatalf("second DB(t) recovered %q, want a duplicate-name panic", msg)
		}
	}()
	testkit.DB(t)
}

func TestDBConnectsOnlyToTheTestContainer(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	conn := db.Config().ConnConfig
	if conn.Port < 54323 || conn.Port > 54338 {
		t.Fatalf("testkit.DB connected to port %d, want a test container port, 54323 to 54338", conn.Port)
	}
	var port int
	var name string
	if err := db.QueryRow(context.Background(),
		`SELECT current_setting('port')::int, current_database()`).Scan(&port, &name); err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^t_[a-z0-9]+_testdbconnectsonlytothetestcontainer_[a-z0-9]+$`).MatchString(name) ||
		name != conn.Database {
		t.Fatalf("database %q (pool says %q), want t_<run>_<test name>_<suffix>", name, conn.Database)
	}
}

func TestDBClonesTheMigratedSchema(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	var tables, revisions int
	err := db.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM pg_tables WHERE schemaname = 'public' AND tablename IN ('events', 'event_deliveries')),
		       (SELECT count(*) FROM atlas_schema_revisions.atlas_schema_revisions)`).Scan(&tables, &revisions)
	if err != nil {
		t.Fatal(err)
	}
	if tables != 2 || revisions == 0 {
		t.Fatalf("clone has %d platform tables and %d atlas revisions", tables, revisions)
	}
}

func TestResetEmptiesEveryTableButAtlasRevisions(t *testing.T) {
	t.Parallel()
	db := testkit.DB(t)
	insertEvents(t, db, testkit.NewIDs(1), 2, "reset")
	testkit.Reset(t, db)
	total, _ := countEvents(t, db, "reset")
	var revisions int
	err := db.QueryRow(context.Background(),
		`SELECT count(*) FROM atlas_schema_revisions.atlas_schema_revisions`).Scan(&revisions)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || revisions == 0 {
		t.Fatalf("after Reset: %d events, %d atlas revisions", total, revisions)
	}
	insertEvents(t, db, testkit.NewIDs(1), 2, "reset")
	if total, _ = countEvents(t, db, "reset"); total != 2 {
		t.Fatalf("the same ids after Reset gave %d events, want 2", total)
	}
}
