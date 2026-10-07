package agents

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

type queueHarness struct {
	q     *checkQueue
	dead  map[int]bool
	clock time.Time
}

type waiter struct {
	q    *checkQueue
	tick chan time.Time
	done chan awaited
	out  *bytes.Buffer
}

func (h *queueHarness) wait(ctx context.Context, name string) *waiter {
	tick := make(chan time.Time)
	q := *h.q
	q.after = func(time.Duration) <-chan time.Time { return tick }
	w := &waiter{q: &q, tick: tick, done: make(chan awaited, 1), out: &bytes.Buffer{}}
	go func() { w.done <- awaited{q.await(ctx, name, w.out), w.out} }()
	return w
}

func (w *waiter) spin(n int) {
	for range n {
		w.tick <- time.Time{}
	}
}

func (w *waiter) pump() awaited {
	for {
		select {
		case w.tick <- time.Time{}:
		case got := <-w.done:
			return got
		}
	}
}

func newQueueHarness(t *testing.T, slots int) *queueHarness {
	t.Helper()
	h := &queueHarness{dead: map[int]bool{}, clock: time.Unix(1_800_000_000, 0)}
	h.q = &checkQueue{
		class: "db", dir: filepath.Join(t.TempDir(), "check-queue"), slots: func() int { return slots },
		alive: func(pid int) bool { return !h.dead[pid] },
		after: func(time.Duration) <-chan time.Time { return nil },
		now: func() time.Time {
			h.clock = h.clock.Add(time.Millisecond)
			return h.clock
		},
	}
	return h
}

func (h *queueHarness) take(t *testing.T, pid int) string {
	t.Helper()
	name, err := h.q.take("/work/"+strings.Repeat("w", pid%7), pid)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func (h *queueHarness) takeIn(t *testing.T, pid int, worktree string) string {
	t.Helper()
	name, err := h.q.take(worktree, pid)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func (w *waiter) stillWaits(t *testing.T, what string) {
	t.Helper()
	w.spin(8)
	select {
	case got := <-w.done:
		t.Fatalf("%s ran: %v", what, got.err)
	default:
	}
}

type awaited struct {
	err error
	out *bytes.Buffer
}

func TestCheckQueue_aCheckRunsOnlyWhenItIsAmongTheFirstSlotsLiveTickets(t *testing.T) {
	t.Parallel()
	h := newQueueHarness(t, 2)
	first, second := h.take(t, 101), h.take(t, 102)
	third := h.take(t, 103)
	later := h.take(t, 104)
	for _, name := range []string{first, second} {
		if err := h.q.await(context.Background(), name, &bytes.Buffer{}); err != nil {
			t.Fatalf("%s should run: %v", name, err)
		}
	}
	waiting, laterWaiting := h.wait(context.Background(), third), h.wait(context.Background(), later)
	waiting.spin(8)
	laterWaiting.spin(8)
	select {
	case got := <-waiting.done:
		t.Fatalf("the third check ran with two earlier tickets: %v", got.err)
	case got := <-laterWaiting.done:
		t.Fatalf("a later arrival ran: %v", got.err)
	default:
	}

	h.q.drop(first)
	if got := waiting.pump(); got.err != nil {
		t.Fatalf("the third check runs once a slot frees: %v", got.err)
	}
	laterWaiting.spin(8)
	select {
	case got := <-laterWaiting.done:
		t.Fatalf("the fourth check ran ahead of the second ticket's release: %v", got.err)
	default:
	}
	h.q.drop(second)
	if got := laterWaiting.pump(); got.err != nil {
		t.Fatalf("the fourth check runs after the second frees: %v", got.err)
	}
	if want := "waiting for a db token: position 3 of 4\n"; waiting.out.String() != want {
		t.Fatalf("third printed %q, want %q", waiting.out, want)
	}
	if want := "waiting for a db token: position 4 of 4\nwaiting for a db token: position 3 of 3\n"; laterWaiting.out.String() != want {
		t.Fatalf("later printed %q, want %q", laterWaiting.out, want)
	}
}

func TestCheckQueue_aLaneRunsOneCheckAtATimeAndLaterLanesPassItsSecond(t *testing.T) {
	t.Parallel()
	h := newQueueHarness(t, 2)
	a1 := h.takeIn(t, 101, "/repo/.worktrees/3462")
	a2 := h.takeIn(t, 102, "/repo/.worktrees/3462-access-pushes")
	b1 := h.takeIn(t, 103, "/repo/.worktrees/3500-other")
	for _, name := range []string{a1, b1} {
		if err := h.q.await(context.Background(), name, &bytes.Buffer{}); err != nil {
			t.Fatalf("%s should run: %v", name, err)
		}
	}
	w := h.wait(context.Background(), a2)
	w.stillWaits(t, "the lane's second check, beside its first")
	h.q.drop(a1)
	if got := w.pump(); got.err != nil {
		t.Fatalf("the lane's second check runs once its first finishes: %v", got.err)
	}
	if want := "waiting for a db token: position 2 of 3; this lane already runs a check\n"; w.out.String() != want {
		t.Fatalf("printed %q, want %q", w.out, want)
	}
}

func (h *queueHarness) admitted(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := h.q.await(context.Background(), name, &bytes.Buffer{}); err != nil {
			t.Fatalf("%s should run: %v", name, err)
		}
	}
}

func TestCheckQueue_aLoneLaneWithFreeSlotsRunsSeveralChecksAtOnce(t *testing.T) {
	t.Parallel()
	h := newQueueHarness(t, 4)
	a := h.takeIn(t, 101, "/repo/.worktrees/3462")
	b := h.takeIn(t, 102, "/repo/.worktrees/3462-b")
	c := h.takeIn(t, 103, "/repo/.worktrees/3462-c")
	h.admitted(t, a, b, c)
}

func TestCheckQueue_aLaneSecondCheckRunsOnceNoOtherLaneIsWaiting(t *testing.T) {
	t.Parallel()
	h := newQueueHarness(t, 3)
	a1 := h.takeIn(t, 101, "/repo/.worktrees/3462")
	a2 := h.takeIn(t, 102, "/repo/.worktrees/3462-b")
	b1 := h.takeIn(t, 103, "/repo/.worktrees/3500")
	h.admitted(t, a1, b1, a2)
}

func TestCheckQueue_anAdmittedCheckKeepsItsSlotWhenANewLaneArrives(t *testing.T) {
	t.Parallel()
	h := newQueueHarness(t, 2)
	a1 := h.takeIn(t, 101, "/repo/.worktrees/3462")
	a2 := h.takeIn(t, 102, "/repo/.worktrees/3462-b")
	h.admitted(t, a1, a2)
	c1 := h.takeIn(t, 103, "/repo/.worktrees/3500")
	w := h.wait(context.Background(), c1)
	w.stillWaits(t, "a new lane's check, with both slots held")
	h.q.drop(a1)
	if got := w.pump(); got.err != nil {
		t.Fatalf("the new lane's check runs once a slot frees: %v", got.err)
	}
}

func TestCheckQueue_aTicketThatCannotRecordItsAdmissionFails(t *testing.T) {
	t.Parallel()
	h := newQueueHarness(t, 1)
	if err := h.q.admit("missing"); err == nil {
		t.Fatal("recording an admission on a missing ticket fails")
	}
}

func TestCheckQueue_aLiveTicketThatCannotBeReadIsSkipped(t *testing.T) {
	t.Parallel()
	h := newQueueHarness(t, 1)
	mine := h.takeIn(t, os.Getpid(), "/repo/.worktrees/1")
	if err := os.Mkdir(filepath.Join(h.q.dir, "0-"+strconv.Itoa(os.Getpid())), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := h.q.await(context.Background(), mine, &bytes.Buffer{}); err != nil {
		t.Fatalf("an unreadable ticket ahead does not hold the slot: %v", err)
	}
}

func TestLane_isTheLeadingDigitsUnderWorktreesAndElseTheWholePath(t *testing.T) {
	t.Parallel()
	for worktree, want := range map[string]string{
		"/repo/.worktrees/3462":               "3462",
		"/repo/.worktrees/3462-access-pushes": "3462",
		"/repo/.worktrees/scratch":            "/repo/.worktrees/scratch",
		"/repo":                               "/repo",
		"/repo/3462":                          "/repo/3462",
	} {
		if got := lane(worktree); got != want {
			t.Errorf("lane(%q) = %q, want %q", worktree, got, want)
		}
	}
}

func TestCheckQueue_aWaitingCheckTakesATokenAddedToTheLocalConfigMidWait(t *testing.T) {
	t.Parallel()
	f := newFixtureFrom(t, rootedRepo)
	env := f.Env(t)
	local := filepath.Join(env.Common, localConfigPath)
	writeFile(t, local, "[check.tokens]\ndb = 2\ncpu = 1\n")
	env = f.Env(t)
	h := newQueueHarness(t, 0)
	q := env.checkQueue("db")
	q.dir, q.alive, q.now = h.q.dir, h.q.alive, h.q.now
	h.q = q
	h.take(t, 101)
	h.take(t, 102)
	third := h.take(t, 103)
	writeFile(t, local, "[check.tokens]\ndb = 0\n")
	w := h.wait(context.Background(), third)
	w.spin(4)
	select {
	case got := <-w.done:
		t.Fatalf("an unreadable local config dropped the startup token count: %v", got.err)
	default:
	}
	writeFile(t, local, "[check.tokens]\ndb = 3\n")
	for polls := 0; ; polls++ {
		select {
		case got := <-w.done:
			if got.err != nil {
				t.Fatalf("the third check runs once check.tokens.db rises to 3: %v", got.err)
			}
		case w.tick <- time.Time{}:
			if polls == 1 {
				t.Fatal("the third check still waits after check.tokens.db rose to 3")
			}
			continue
		}
		break
	}
	if want := "waiting for a db token: position 3 of 3\n"; w.out.String() != want {
		t.Fatalf("printed %q, want %q", w.out, want)
	}
	if env.tokens("cpu") != 6 {
		t.Fatalf("the cpu class rereads its own count: %d", env.tokens("cpu"))
	}
}

func TestCheckQueue_eachClassHasItsOwnQueueSoCPURowsRunWhileADatabaseRowWaits(t *testing.T) {
	t.Parallel()
	db, cpu := newQueueHarness(t, 1), newQueueHarness(t, 2)
	cpu.q.class = "cpu"
	a1 := db.takeIn(t, 101, "/repo/.worktrees/3462")
	b1 := db.takeIn(t, 102, "/repo/.worktrees/3500")
	a2, b2 := cpu.takeIn(t, 101, "/repo/.worktrees/3462"), cpu.takeIn(t, 102, "/repo/.worktrees/3500")
	for _, name := range []string{a2, b2} {
		if err := cpu.q.await(context.Background(), name, &bytes.Buffer{}); err != nil {
			t.Fatalf("both checks hold a cpu token together: %v", err)
		}
	}
	if err := db.q.await(context.Background(), a1, &bytes.Buffer{}); err != nil {
		t.Fatalf("the first database row runs: %v", err)
	}
	w := db.wait(context.Background(), b1)
	w.stillWaits(t, "the second database row, beside the first")
	db.q.drop(a1)
	if got := w.pump(); got.err != nil {
		t.Fatalf("the second database row runs after the first drops: %v", got.err)
	}
	if want := "waiting for a db token: position 2 of 2\n"; w.out.String() != want {
		t.Fatalf("printed %q, want %q", w.out, want)
	}
	if got := cpu.q.dir; got == db.q.dir {
		t.Fatal("two classes share one queue directory")
	}
}

func TestCheckQueue_aTicketWhosePIDIsDeadIsReclaimed(t *testing.T) {
	t.Parallel()
	h := newQueueHarness(t, 1)
	dead := h.take(t, 201)
	live := h.take(t, 202)
	h.dead[201] = true
	if err := h.q.await(context.Background(), live, &bytes.Buffer{}); err != nil {
		t.Fatalf("a dead holder frees its slot: %v", err)
	}
	if _, err := os.Stat(filepath.Join(h.q.dir, dead)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the dead ticket stays: %v", err)
	}
	junk := filepath.Join(h.q.dir, "not-a-ticket")
	if err := os.WriteFile(junk, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := h.q.standing(live); err != nil || got.total != 1 {
		t.Fatalf("a malformed name is reclaimed: %d %v", got.total, err)
	}
}

func TestCheckQueue_cancellingTheWaitReturnsAndALostTicketFails(t *testing.T) {
	t.Parallel()
	h := newQueueHarness(t, 1)
	h.take(t, 301)
	waiterTicket := h.take(t, 302)
	ctx, cancel := context.WithCancel(context.Background())
	w := h.wait(ctx, waiterTicket)
	w.spin(1)
	cancel()
	if got := <-w.done; !errors.Is(got.err, context.Canceled) {
		t.Fatalf("cancelled wait: %v", got.err)
	}
	h.q.drop(waiterTicket)
	if err := h.q.await(context.Background(), waiterTicket, &bytes.Buffer{}); !errors.Is(err, errTicketLost) {
		t.Fatalf("lost ticket: %v", err)
	}
	if err := os.RemoveAll(h.q.dir); err != nil {
		t.Fatal(err)
	}
	if _, err := h.q.standing(waiterTicket); err == nil {
		t.Fatal("an unreadable queue directory fails")
	}
}

func TestCheckQueue_standingFailsWhenTheQueueDirectoryIsAFile(t *testing.T) {
	t.Parallel()
	h := newQueueHarness(t, 1)
	if err := os.RemoveAll(h.q.dir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, h.q.dir, "not a directory")
	if _, err := h.q.standing("ticket"); err == nil || !strings.Contains(err.Error(), "read the stage 0 queue") {
		t.Fatalf("a queue path that is a file: %v", err)
	}
}

func TestCheckQueue_pidAliveSeesThisProcessAndNotAnExitedOne(t *testing.T) {
	t.Parallel()
	if !pidAlive(os.Getpid()) {
		t.Fatal("this process is alive")
	}
	if pidAlive(1 << 30) {
		t.Fatal("a pid nothing owns is dead")
	}
}

func (h *checkHarness) backendChange(t *testing.T, name string) {
	t.Helper()
	h.commit(t, map[string]string{"apps/backend/internal/a/" + name + ".go": "package a\n"})
	h.affected = "./internal/a\n"
}

func TestCheck_takesATicketPerClassedRowAndRemovesItAfterwards(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.backendChange(t, "a")
	if code, stdout, stderr := h.check(t); code != 0 || strings.Contains(stdout, "waiting") {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	queue := filepath.Join(h.Env(t).Common, ".monaco", "check-queue")
	for _, class := range []string{"db", "cpu"} {
		left, err := os.ReadDir(filepath.Join(queue, class))
		if err != nil || len(left) != 0 {
			t.Fatalf("%s tickets left after the run: %v %v", class, left, err)
		}
	}

	h.backendChange(t, "b")
	if err := os.RemoveAll(queue); err != nil {
		t.Fatal(err)
	}
	writeFile(t, queue, "not a directory\n")
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "take a stage 0 ticket") {
		t.Fatalf("an unwritable queue fails the run: %d %q", code, stderr)
	}
}

func TestCheck_aFreshRunTakesTicketsToo(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.backendChange(t, "a")
	if code, _, stderr := h.check(t); code != 0 {
		t.Fatalf("first: %d %q", code, stderr)
	}
	queue := filepath.Join(h.Env(t).Common, ".monaco", "check-queue")
	if err := os.RemoveAll(queue); err != nil {
		t.Fatal(err)
	}
	writeFile(t, queue, "not a directory\n")
	if code, _, stderr := h.check(t, "--fresh"); code != 1 || !strings.Contains(stderr, "take a stage 0 ticket") {
		t.Fatalf("--fresh skipped the stage 0 queue: %d %q", code, stderr)
	}
	if err := os.Remove(queue); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := h.check(t, "--fresh"); code != 0 || !strings.Contains(stdout, "passed") {
		t.Fatalf("--fresh: %d %q %q", code, stdout, stderr)
	}
	if left, err := os.ReadDir(filepath.Join(queue, "cpu")); err != nil || len(left) != 0 {
		t.Fatalf("--fresh left a ticket or took none: %v %v", left, err)
	}
}

func TestCheck_aCarriedRunTakesNoTicket(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.onPRBranch(t, map[string]string{"x.sh": "echo\n"})
	if code, _, stderr := h.check(t); code != 0 {
		t.Fatalf("first: %d %q", code, stderr)
	}
	h.moveBase(t, map[string]string{"y.txt": "y\n"})
	h.rebase(t)
	queue := filepath.Join(h.Env(t).Common, ".monaco", "check-queue")
	if err := os.RemoveAll(queue); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := h.check(t)
	if code != 0 || !strings.Contains(stdout, "carried") {
		t.Fatalf("carry: %d %q %q", code, stdout, stderr)
	}
	if _, err := os.Stat(queue); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a carried check made a ticket: %v", err)
	}
}

func TestTakeToken_failsWhenTheQueueCannotBeWrittenOrTheWaitIsCancelled(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	env := h.Env(t)
	writeFile(t, filepath.Join(env.Common, ".monaco"), "not a directory\n")
	if _, err := env.takeToken(context.Background(), "db", &bytes.Buffer{}); err == nil ||
		!strings.Contains(cliText(err), "take a stage 0 ticket") {
		t.Fatalf("an unwritable queue fails the check: %v", err)
	}

	if err := os.Remove(filepath.Join(env.Common, ".monaco")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(env.Common, localConfigPath), "[check.tokens]\ndb = 1\n")
	queue := filepath.Join(env.Common, ".monaco", "check-queue", "db")
	if err := os.MkdirAll(queue, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, pid := range []int{os.Getpid(), os.Getppid()} {
		writeFile(t, filepath.Join(queue, "1-"+strconv.Itoa(pid)), "/other/"+strconv.Itoa(pid)+"\n")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if _, err := env.takeToken(ctx, "db", &out); !errors.Is(err, context.Canceled) ||
		!strings.Contains(out.String(), "waiting for a db token: position 3 of 3") {
		t.Fatalf("a cancelled wait: %v %q", err, out.String())
	}
	left, _ := os.ReadDir(queue)
	if len(left) != 2 {
		t.Fatalf("the cancelled wait left its ticket: %v", left)
	}
}

func TestCheck_aTokenWaitIsPrintedLoggedAndNotChargedToTheRowBudget(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	env := h.Env(t)
	env.Actions = true
	env.Config.Budget["go"] = 10 * time.Second
	writeFile(t, filepath.Join(env.Common, localConfigPath), "[check.tokens]\ncpu = 1\n")
	env.Now = func() time.Time { return h.clock }
	holder := filepath.Join(env.Common, ".monaco", "check-queue", "cpu", "1-"+strconv.Itoa(os.Getppid()))
	writeFile(t, holder, "/other/.worktrees/9\n")
	env.After = func(time.Duration) <-chan time.Time {
		h.clock = h.clock.Add(30 * time.Second)
		_ = os.Remove(holder)
		tick := make(chan time.Time, 1)
		tick <- h.clock
		return tick
	}
	run := &checkRun{env: env, start: env.Now()}
	var out bytes.Buffer
	row := checkRow{label: "go vet", kind: "go", dir: env.Work, class: "cpu", cmds: [][]string{{"vet"}}}
	if err := run.row(context.Background(), row, &out); err != nil {
		t.Fatalf("a 30 s wait fails a 10 s budget: %v\n%s", err, out.String())
	}
	for _, want := range []string{
		"waiting for a cpu token: position 2 of 2", "go vet          ok    1.0s  waited 30s for a cpu token",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout lacks %q: %q", want, out.String())
		}
	}
	if !strings.Contains(run.log.String(), "go vet: waited 30s for a cpu token\n") {
		t.Errorf("log lacks the wait: %q", run.log.String())
	}
}

func TestCheck_aDatabaseRowThatCannotTakeATestDatabaseReleasesItsToken(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	env := h.Env(t)
	writeFile(t, filepath.Join(env.Common, ".monaco", "test-db"), "not a directory\n")
	run := &checkRun{env: env, start: env.Now()}
	row := checkRow{
		label: "go test -short", kind: "go", dir: env.Work, class: "db",
		dbCmds: func(testDB) [][]string { return nil },
	}
	if err := run.row(context.Background(), row, &bytes.Buffer{}); err == nil {
		t.Fatal("no test database slot fails the row")
	}
	if n := ticketCount(t, env, "db"); n != 0 {
		t.Fatalf("%d db tickets left", n)
	}
}

func TestCheck_aFailingTestDatabaseStartFailsTheRowBeforeItsCommands(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"apps/backend/internal/a/a.go": "package a\n"})
	h.affected = "./internal/a\n"
	h.replies = []reply{{prefix: "docker compose", err: errors.New("no docker")}}
	code, stdout, _ := h.check(t)
	if code != 1 || !strings.Contains(stdout, "go test -short  FAIL  docker compose") ||
		slices.ContainsFunc(h.calls, func(c string) bool { return strings.Contains(c, "go test ") }) {
		t.Fatalf("code=%d stdout=%q calls=%v", code, stdout, h.calls)
	}
}
