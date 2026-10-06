package agents

import (
	"slices"
	"strings"
	"testing"
)

const (
	oldMigration   = "apps/backend/migrations/20261001000000_a.sql"
	newestOnTrunk  = "apps/backend/migrations/20261005160148_funding.sql"
	renamedAddedBy = "apps/backend/migrations/20261005160149_a.sql"
	migrationsDiff = "diff --name-only base..origin/fb -- " + migrationsDir
)

func addsMigration(name string) []File {
	return []File{{Filename: name, Status: "added"}, {Filename: migrationsDir + "atlas.sum", Status: "modified"}}
}

func migrationStacks(t *testing.T) (*fixture, *stackGH) {
	t.Helper()
	f := newFixture(t)
	q := labeled(green(t, 7, "q2", "fb"), "merge-queue")
	s := gateStack(t, f, map[int][]File{
		2: addsMigration(oldMigration),
		7: addsMigration(migrationsDir + "20261002000000_b.sql"),
	}, q)
	s.gitOut[migrationsDiff] = ""
	s.gitOut["ls-tree --name-only origin/fb "+migrationsDir] = migrationsDir + "atlas.sum\n" + newestOnTrunk + "\n"
	s.gitOut["mv "+oldMigration+" "+renamedAddedBy] = ""
	f.noFailures()
	return f, s
}

func TestMigrationWait_armsBehindAQueuedStackThatAlsoAddsOne(t *testing.T) {
	t.Parallel()
	f, s := migrationStacks(t)
	heldFor(t, f, s, "waiting on queued stack #7, which also adds a migration")
	if code, stdout, stderr := f.agents(t, "watch", "--once"); code != 0 || f.owned(t).Armed == nil ||
		s.prs[2].labeled("merge-queue") || strings.Contains(stdout, "restacked") {
		t.Fatalf("a waiting stack: %d %q %q", code, stdout, stderr)
	}
	if s.ran("bash scripts/restack-regen.sh") {
		t.Fatal("restacked before the other stack landed")
	}
}

func TestMigrationWait_aStackWhoseOthersAddNoMigrationLandsAtOnce(t *testing.T) {
	t.Parallel()
	f, s := migrationStacks(t)
	f.hub.on(list("/pulls/7/files?"), []File{{Filename: "README.md", Status: "added"}})
	if code, stdout, stderr := f.agents(t, "land-stack", "2"); code != 0 || !s.prs[2].labeled("merge-queue") {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
}

func TestMigrationWait_restacksRenumbersAndResubmitsOnceTheOtherStackLanded(t *testing.T) {
	t.Parallel()
	f, s := migrationStacks(t)
	heldFor(t, f, s, "waiting on queued stack #7")
	s.prs[7].Labels.Nodes = nil
	s.gitOut[migrationsDiff] = migrationsDir + "atlas.sum\n" + newestOnTrunk + "\n"
	code, stdout, stderr := f.agents(t, "watch", "--once")
	want := "armed stack #2 restacked over staging's migrations and resubmitted; waiting for stage 1"
	if code != 0 || !strings.Contains(stdout, want) || f.owned(t).Armed == nil {
		t.Fatalf("%d %q %q", code, stdout, stderr)
	}
	for _, call := range []string{
		"bash scripts/restack-regen.sh", "git mv " + oldMigration + " " + renamedAddedBy, "go generate ./...",
		"gt modify --all --no-interactive", "go run ./cmd/monacoctl agents check",
		"gt submit --stack --no-interactive --draft",
	} {
		if !s.ran(call) {
			t.Errorf("never ran %q", call)
		}
	}
}

func TestMigrationWait_leavesAMigrationAboveStagingsNewestAlone(t *testing.T) {
	t.Parallel()
	f, s := migrationStacks(t)
	heldFor(t, f, s, "waiting on queued stack #7")
	s.prs[7].Labels.Nodes = nil
	f.hub.on(list("/pulls/2/files?"), addsMigration(migrationsDir+"20261006000000_a.sql"))
	s.gitOut[migrationsDiff] = newestOnTrunk + "\n"
	if code, stdout, stderr := f.agents(t, "watch", "--once"); code != 0 || !s.ran("gt submit") ||
		s.ran("git mv") || s.ran("go generate") {
		t.Fatalf("%d %q %q %q", code, stdout, stderr, s.gitCalls)
	}
}

func TestMigrationWait_aFailedRestackDisarmsWithTheReason(t *testing.T) {
	t.Parallel()
	f, s := migrationStacks(t)
	heldFor(t, f, s, "waiting on queued stack #7")
	s.prs[7].Labels.Nodes = nil
	s.gitOut[migrationsDiff] = newestOnTrunk + "\n"
	s.fail = "gt sync"
	if code, stdout, _ := f.agents(t, "watch", "--once"); !strings.Contains(stdout, "disarmed: migration restack:") ||
		f.owned(t).Armed != nil {
		t.Fatalf("%d %q", code, stdout)
	}
}

func (s *stackGH) ran(prefix string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.calls {
		if strings.Contains(c.line, prefix) {
			return true
		}
	}
	return slices.ContainsFunc(s.gitCalls, func(c string) bool { return strings.Contains("git "+c, prefix) })
}
