package agents

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

const (
	testDBPort  = 54323
	testDBSlots = 16
	slotTmpfs   = "1536m"
	maxTestP    = 4
	perSlotVar  = "MONACO_TEST_DB_PORT"
)

const dropLeftoverClones = `psql -U "$POSTGRES_USER" -Atc "SELECT format('DROP DATABASE %I WITH (FORCE);', datname) ` +
	`FROM pg_database WHERE datname LIKE 't\_%' OR datname LIKE '%\_inst\_%'" | psql -U "$POSTGRES_USER" -q`

const dropDeadRunClones = `SELECT format('DROP DATABASE IF EXISTS %I WITH (FORCE);', datname) FROM pg_database ` +
	`WHERE datname LIKE 't\_%' AND split_part(datname, '_', 2) NOT IN ` +
	`(SELECT split_part(datname, '_', 2) FROM pg_stat_activity WHERE datname LIKE 't\_%')`

var errSlotsHeld = errors.New("every test database slot is held")

type testDB struct {
	slot int
	busy int
}

func (db testDB) port() int { return testDBPort + db.slot }

func (db testDB) url() string {
	port := func(p int) string { return ":" + strconv.Itoa(p) + "/" }
	return strings.Replace(config.DefaultTestDBURL, port(testDBPort), port(db.port()), 1)
}

func (db testDB) testEnv() []string { return []string{"env", "TEST_DATABASE_URL=" + db.url()} }

func (db testDB) row(work string) checkRow {
	project, vars := "monaco", []string(nil)
	if db.slot > 0 {
		project = fmt.Sprintf("monaco-test-%d", db.slot)
		vars = append(vars, "env", fmt.Sprintf("MONACO_TEST_DB_NAME=monaco-postgres-test-%d", db.slot),
			perSlotVar+"="+strconv.Itoa(db.port()), "MONACO_TEST_DB_TMPFS="+slotTmpfs)
	}
	row := checkRow{label: "test db", kind: "go", dir: work, cmds: [][]string{slices.Concat(vars, []string{
		"docker", "compose", "-p", project, "--profile", "test", "up", "-d", "--wait", "postgres-test",
	})}}
	container, drop := "monaco-postgres-test", `psql -U "$POSTGRES_USER" -Atc "`+dropDeadRunClones+`" | psql -U "$POSTGRES_USER" -q`
	if db.slot > 0 {
		container, drop = fmt.Sprintf("monaco-postgres-test-%d", db.slot), dropLeftoverClones
	}
	row.cmds = append(row.cmds, []string{"docker", "exec", container, "sh", "-c", drop})
	return row
}

func (env *Env) takeTestDB() (testDB, func(), error) {
	dir := filepath.Join(env.Common, ".monaco", "test-db")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return testDB{}, nil, fmt.Errorf("take a test database slot: %w", err)
	}
	for slot := range testDBSlots {
		held, ok, err := tryLock(filepath.Join(dir, strconv.Itoa(slot)))
		if err != nil {
			return testDB{}, nil, fmt.Errorf("take a test database slot: %w", err)
		}
		if !ok {
			continue
		}
		release := func() { _ = held.Close() }
		busy, err := busySlots(dir)
		if err != nil {
			release()
			return testDB{}, nil, err
		}
		db := testDB{slot: slot, busy: busy}
		if !env.perSlotTestDB() {
			db.slot = 0
		}
		return db, release, nil
	}
	return testDB{}, nil, fmt.Errorf("%w: %d in %s", errSlotsHeld, testDBSlots, dir)
}

func busySlots(dir string) (int, error) {
	busy := 0
	for slot := range testDBSlots {
		f, ok, err := tryLock(filepath.Join(dir, strconv.Itoa(slot)))
		if err != nil {
			return 0, fmt.Errorf("count test database slots: %w", err)
		}
		if !ok {
			busy++
			continue
		}
		_ = f.Close()
	}
	return busy, nil
}

func (env *Env) perSlotTestDB() bool {
	compose, err := os.ReadFile(filepath.Join(env.Work, "apps", "backend", "deployments", "compose.yml"))
	return err == nil && strings.Contains(string(compose), perSlotVar)
}
