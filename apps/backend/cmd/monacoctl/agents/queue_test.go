package agents

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
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
		dir: filepath.Join(t.TempDir(), "check-queue"), slots: slots,
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
	if want := "waiting for a stage 0 slot: position 3 of 4\n"; waiting.out.String() != want {
		t.Fatalf("third printed %q, want %q", waiting.out, want)
	}
	if want := "waiting for a stage 0 slot: position 4 of 4\nwaiting for a stage 0 slot: position 3 of 3\n"; laterWaiting.out.String() != want {
		t.Fatalf("later printed %q, want %q", laterWaiting.out, want)
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
	if _, total, err := h.q.standing(live); err != nil || total != 1 {
		t.Fatalf("a malformed name is reclaimed: %d %v", total, err)
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
	if _, _, err := h.q.standing(waiterTicket); err == nil {
		t.Fatal("an unreadable queue directory fails")
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

func TestCheck_takesATicketForTheRunAndRemovesItAfterwards(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"x.sh": "echo\n"})
	if code, stdout, stderr := h.check(t); code != 0 || strings.Contains(stdout, "waiting") {
		t.Fatalf("check: %d %q %q", code, stdout, stderr)
	}
	queue := filepath.Join(h.Env(t).Common, ".monaco", "check-queue")
	left, err := os.ReadDir(queue)
	if err != nil || len(left) != 0 {
		t.Fatalf("tickets left after the run: %v %v", left, err)
	}

	h.commit(t, map[string]string{"y.sh": "echo\n"})
	if err := os.RemoveAll(queue); err != nil {
		t.Fatal(err)
	}
	writeFile(t, queue, "not a directory\n")
	if code, _, stderr := h.check(t); code != 1 || !strings.Contains(stderr, "take a stage 0 ticket") {
		t.Fatalf("an unwritable queue fails the run: %d %q", code, stderr)
	}
}

func TestCheck_aFreshRunTakesNoTicketAndWaitsForNoSlot(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	h.commit(t, map[string]string{"x.sh": "echo\n"})
	env := h.Env(t)
	queue := filepath.Join(env.Common, ".monaco", "check-queue")
	for _, pid := range []int{os.Getpid(), os.Getppid()} {
		if err := os.MkdirAll(queue, 0o750); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(queue, "1-"+strconv.Itoa(pid)), "/other\n")
	}
	if code, stdout, stderr := h.check(t, "--fresh"); code != 0 || strings.Contains(stdout, "waiting") {
		t.Fatalf("--fresh waited: %d %q %q", code, stdout, stderr)
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

func TestTakeSlot_failsWhenTheQueueCannotBeWrittenOrTheWaitIsCancelled(t *testing.T) {
	t.Parallel()
	h := newCheckHarness(t)
	env := h.Env(t)
	writeFile(t, filepath.Join(env.Common, ".monaco"), "not a directory\n")
	if _, _, err := env.takeSlot(context.Background(), &bytes.Buffer{}); err == nil ||
		!strings.Contains(cliText(err), "take a stage 0 ticket") {
		t.Fatalf("an unwritable queue fails the check: %v", err)
	}

	if err := os.Remove(filepath.Join(env.Common, ".monaco")); err != nil {
		t.Fatal(err)
	}
	queue := filepath.Join(env.Common, ".monaco", "check-queue")
	if err := os.MkdirAll(queue, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, pid := range []int{os.Getpid(), os.Getppid()} {
		writeFile(t, filepath.Join(queue, "1-"+strconv.Itoa(pid)), "/other\n")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if _, _, err := env.takeSlot(ctx, &out); !errors.Is(err, context.Canceled) ||
		!strings.Contains(out.String(), "position 3 of 3") {
		t.Fatalf("a cancelled wait: %v %q", err, out.String())
	}
	left, _ := os.ReadDir(queue)
	if len(left) != 2 {
		t.Fatalf("the cancelled wait left its ticket: %v", left)
	}
}
