package qa_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// lockEnv is one scratch lock setup: both classes point into a temp dir, so a test never
// touches /private/tmp or another test's lock. Each class has one slot unless a call's
// extraEnv sets more, so the RAM-sized default never decides a test.
type lockEnv struct {
	t      *testing.T
	dir    string
	script string
	extra  []string
	// pgroup starts each call in its own process group, so a test can signal the group the
	// way a terminal's Ctrl-C does.
	pgroup bool
	// calls numbers the callers, so each gets its own lane unless a test names one.
	calls atomic.Int32
}

func newLockEnv(t *testing.T) *lockEnv {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return &lockEnv{
		t:      t,
		dir:    t.TempDir(),
		script: filepath.Join(wd, "xcode-lock.sh"),
	}
}

func (e *lockEnv) xcodeLock() string   { return filepath.Join(e.dir, "xcode.lock") }
func (e *lockEnv) swiftpmLock() string { return filepath.Join(e.dir, "swiftpm.lock") }

// call is one running xcode-lock.sh process.
type call struct {
	cmd    *exec.Cmd
	stderr *syncBuffer
	done   chan struct{}
	err    error
}

// syncBuffer is a bytes.Buffer that the test can read while the process still writes.
type syncBuffer struct {
	mu chan struct{}
	b  bytes.Buffer
}

func newSyncBuffer() *syncBuffer { return &syncBuffer{mu: make(chan struct{}, 1)} }

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu <- struct{}{}
	defer func() { <-s.mu }()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu <- struct{}{}
	defer func() { <-s.mu }()
	return s.b.String()
}

// start runs xcode-lock.sh with args from workdir. extraEnv entries are KEY=VALUE.
func (e *lockEnv) start(workdir string, extraEnv []string, args ...string) *call {
	e.t.Helper()
	cmd := exec.Command(e.script, args...)
	cmd.Dir = workdir
	if e.pgroup {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	cmd.Env = append(os.Environ(),
		"MONACO_XCODE_LOCK_DIR="+e.xcodeLock(),
		"MONACO_SWIFTPM_LOCK_DIR="+e.swiftpmLock(),
		"MONACO_LOCK_POLL=0.1",
		"MONACO_XCODE_SLOTS=1",
		"MONACO_SWIFTPM_SLOTS=1",
		"MONACO_LANE=lane"+strconv.Itoa(int(e.calls.Add(1))),
	)
	cmd.Env = append(cmd.Env, e.extra...)
	cmd.Env = append(cmd.Env, extraEnv...)
	c := &call{cmd: cmd, stderr: newSyncBuffer(), done: make(chan struct{})}
	cmd.Stderr = c.stderr
	if err := cmd.Start(); err != nil {
		e.t.Fatalf("start %v: %v", args, err)
	}
	go func() {
		c.err = cmd.Wait()
		close(c.done)
	}()
	e.t.Cleanup(func() {
		select {
		case <-c.done:
		default:
			_ = cmd.Process.Kill()
			<-c.done
		}
	})
	return c
}

func (c *call) wait(t *testing.T) int {
	t.Helper()
	select {
	case <-c.done:
	case <-time.After(15 * time.Second):
		t.Fatalf("process did not exit in 15s; stderr:\n%s", c.stderr.String())
	}
	var ee *exec.ExitError
	if errors.As(c.err, &ee) {
		return ee.ExitCode()
	}
	if c.err != nil {
		t.Fatalf("wait: %v", c.err)
	}
	return 0
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// await polls until ok holds. It fails as soon as one of the watched calls exits first, so
// it ends on an event, never on a deadline; a hang is left to go test's -timeout.
func await(t *testing.T, what string, ok func() bool, watched ...*call) {
	t.Helper()
	for !ok() {
		for _, c := range watched {
			select {
			case <-c.done:
				if ok() {
					return
				}
				t.Fatalf("%s: a caller exited first; stderr:\n%s", what, c.stderr.String())
			default:
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func countTickets(queueDir string) int {
	entries, err := os.ReadDir(queueDir)
	if err != nil {
		return 0
	}
	return len(entries)
}

// holdUntil is a shell command that records its start, then holds until release exists.
func holdUntil(started, release string) []string {
	return []string{"sh", "-c", `touch "$1"; while [ ! -e "$2" ]; do sleep 0.05; done`, "sh", started, release}
}

// writeExecutable holds syscall.ForkLock while the file is open for writing, so no fork can copy the
// descriptor. A child that holds the copy makes exec of the file fail with ETXTBSY until the child
// itself execs (golang/go#22315).
func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	syscall.ForkLock.Lock()
	err := os.WriteFile(path, []byte(body), 0o700)
	syscall.ForkLock.Unlock()
	if err != nil {
		t.Fatal(err)
	}
}

func TestXcodeLockArrivalOrder(t *testing.T) {
	t.Parallel()
	e := newLockEnv(t)
	blockerDir := filepath.Join(e.dir, "worktree-blocker")
	if err := os.Mkdir(blockerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	started := filepath.Join(e.dir, "blocker.started")
	release := filepath.Join(e.dir, "blocker.release")
	order := filepath.Join(e.dir, "order")

	blocker := e.start(blockerDir, nil, append([]string{"xcode"}, holdUntil(started, release)...)...)
	eventually(t, "the blocker to hold the lock", func() bool { return exists(started) })

	// Three callers, one at a time, each only after the previous one's ticket is in the queue.
	var waiters []*call
	for i := 1; i <= 3; i++ {
		name := strconv.Itoa(i)
		w := e.start(e.dir, nil, "xcode", "sh", "-c", `echo "$1" >> "$2"`, "sh", name, order)
		waiters = append(waiters, w)
		want := i
		eventually(t, "ticket "+name+" in the queue", func() bool {
			return countTickets(e.xcodeLock()+".queue") == want
		})
	}

	// The second waiter names its place and the holder's worktree.
	eventually(t, "waiter 2 to log its position", func() bool {
		return strings.Contains(waiters[1].stderr.String(), "queue position 2 behind pid ")
	})
	log := waiters[1].stderr.String()
	pidBytes, err := os.ReadFile(filepath.Join(e.xcodeLock(), "pid"))
	if err != nil {
		t.Fatalf("holder pid file: %v", err)
	}
	holderPid := strings.TrimSpace(string(pidBytes))
	if !strings.Contains(log, "behind pid "+holderPid+" (") || !strings.Contains(log, "worktree-blocker") {
		t.Fatalf("waiter log does not name the holder pid %s and worktree:\n%s", holderPid, log)
	}

	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := blocker.wait(t); code != 0 {
		t.Fatalf("blocker exit %d:\n%s", code, blocker.stderr.String())
	}
	for i, w := range waiters {
		if code := w.wait(t); code != 0 {
			t.Fatalf("waiter %d exit %d:\n%s", i+1, code, w.stderr.String())
		}
	}
	got, err := os.ReadFile(order)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "1\n2\n3\n" {
		t.Fatalf("callers ran in order %q, want arrival order 1 2 3", strings.ReplaceAll(string(got), "\n", " "))
	}
	if exists(e.xcodeLock()) {
		t.Fatalf("lock dir still present after every caller exited")
	}
}

func TestXcodeLockTakesOverDeadHolder(t *testing.T) {
	t.Parallel()
	e := newLockEnv(t)

	dead := exec.Command("sh", "-c", "exit 0")
	if err := dead.Run(); err != nil {
		t.Fatal(err)
	}
	deadPid := strconv.Itoa(dead.Process.Pid)
	if err := syscall.Kill(dead.Process.Pid, 0); err == nil {
		t.Fatalf("pid %s is still alive", deadPid)
	}

	// A holder that died with its lock in place, and an older ticket from a dead waiter.
	if err := os.MkdirAll(e.xcodeLock(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.xcodeLock(), "pid"), []byte(deadPid+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	queue := e.xcodeLock() + ".queue"
	if err := os.MkdirAll(queue, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(queue, "1000000000000000000."+deadPid), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	c := e.start(e.dir, nil, "xcode", "true")
	if code := c.wait(t); code != 0 {
		t.Fatalf("exit %d:\n%s", code, c.stderr.String())
	}
	if !strings.Contains(c.stderr.String(), "taking over stale lock from pid "+deadPid) {
		t.Fatalf("no takeover message:\n%s", c.stderr.String())
	}
	if n := countTickets(queue); n != 0 {
		t.Fatalf("%d tickets left in the queue", n)
	}
}

func TestXcodeLockHoldCap(t *testing.T) {
	t.Parallel()
	e := newLockEnv(t)

	c := e.start(e.dir, []string{"MONACO_LOCK_HOLD=1"}, "xcode", "sleep", "30")
	if code := c.wait(t); code != 124 {
		t.Fatalf("exit %d, want 124:\n%s", code, c.stderr.String())
	}
	if !strings.Contains(c.stderr.String(), "command exceeded the 1s hold cap") {
		t.Fatalf("no hold cap message:\n%s", c.stderr.String())
	}
	if exists(e.xcodeLock()) {
		t.Fatalf("lock dir still present after the cap")
	}
}

func TestXcodeLockClassesOverlapAndSameClassSerializes(t *testing.T) {
	t.Parallel()
	e := newLockEnv(t)

	// xcode and swiftpm: each holder waits for the other's marker, so both exit 0 only
	// if the two ran at the same time.
	meet := func(mine, other string) []string {
		return []string{"sh", "-c",
			`touch "$1"; i=0; while [ ! -e "$2" ]; do i=$((i+1)); [ "$i" -gt 200 ] && exit 3; sleep 0.05; done`,
			"sh", mine, other}
	}
	xa := filepath.Join(e.dir, "xcode.in")
	sa := filepath.Join(e.dir, "swiftpm.in")
	x := e.start(e.dir, nil, append([]string{"xcode"}, meet(xa, sa)...)...)
	s := e.start(e.dir, nil, append([]string{"swiftpm"}, meet(sa, xa)...)...)
	if code := x.wait(t); code != 0 {
		t.Fatalf("xcode holder exit %d (never overlapped?):\n%s", code, x.stderr.String())
	}
	if code := s.wait(t); code != 0 {
		t.Fatalf("swiftpm holder exit %d (never overlapped?):\n%s", code, s.stderr.String())
	}

	// Two swiftpm holders: the second starts only after the first has released.
	firstIn := filepath.Join(e.dir, "first.in")
	secondIn := filepath.Join(e.dir, "second.in")
	release := filepath.Join(e.dir, "first.release")
	first := e.start(e.dir, nil, append([]string{"swiftpm"}, holdUntil(firstIn, release)...)...)
	eventually(t, "the first swiftpm holder", func() bool { return exists(firstIn) })
	second := e.start(e.dir, nil, "swiftpm", "touch", secondIn)
	eventually(t, "the second swiftpm caller to queue", func() bool {
		return strings.Contains(second.stderr.String(), "queue position 1 behind pid ")
	})
	time.Sleep(500 * time.Millisecond)
	if exists(secondIn) {
		t.Fatalf("second swiftpm caller ran while the first held the lock")
	}
	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := first.wait(t); code != 0 {
		t.Fatalf("first swiftpm holder exit %d:\n%s", code, first.stderr.String())
	}
	if code := second.wait(t); code != 0 {
		t.Fatalf("second swiftpm caller exit %d:\n%s", code, second.stderr.String())
	}
	if !exists(secondIn) {
		t.Fatalf("second swiftpm caller never ran")
	}
}

func TestXcodeLockCallWithoutClassUsesXcode(t *testing.T) {
	t.Parallel()
	e := newLockEnv(t)

	// The command checks, while it runs, that the xcode lock is held and the swiftpm lock is not.
	script := `test -f "$1/pid" && test ! -e "$2"`
	c := e.start(e.dir, nil, "sh", "-c", script, "sh", e.xcodeLock(), e.swiftpmLock())
	if code := c.wait(t); code != 0 {
		t.Fatalf("exit %d:\n%s", code, c.stderr.String())
	}
	if exists(e.xcodeLock()) {
		t.Fatalf("lock dir still present after the call")
	}

	// A command's own exit code passes through.
	c = e.start(e.dir, nil, "sh", "-c", "exit 7")
	if code := c.wait(t); code != 7 {
		t.Fatalf("exit %d, want 7", code)
	}

	// No command at all is a usage error.
	c = e.start(e.dir, nil, "swiftpm")
	if code := c.wait(t); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

func TestXcodeLockWaitTimeout(t *testing.T) {
	t.Parallel()
	e := newLockEnv(t)
	started := filepath.Join(e.dir, "holder.started")
	release := filepath.Join(e.dir, "holder.release")

	holder := e.start(e.dir, nil, append([]string{"xcode"}, holdUntil(started, release)...)...)
	eventually(t, "the holder", func() bool { return exists(started) })

	waiter := e.start(e.dir, []string{"MONACO_XCODE_LOCK_TIMEOUT=1"}, "xcode", "true")
	if code := waiter.wait(t); code != 75 {
		t.Fatalf("exit %d, want 75:\n%s", code, waiter.stderr.String())
	}
	if n := countTickets(e.xcodeLock() + ".queue"); n != 0 {
		t.Fatalf("%d tickets left after the waiter gave up", n)
	}

	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := holder.wait(t); code != 0 {
		t.Fatalf("holder exit %d", code)
	}
}

// TestXcodeLockHoldCapWithoutTimeoutBinary runs the shell watchdog that stands in for
// `timeout` on a Mac without Homebrew coreutils.
func TestXcodeLockHoldCapWithoutTimeoutBinary(t *testing.T) {
	t.Parallel()
	e := newLockEnv(t)

	e.withoutTimeout()

	c := e.start(e.dir, []string{"MONACO_LOCK_HOLD=1"}, "xcode", "sleep", "30")
	if code := c.wait(t); code != 124 {
		t.Fatalf("exit %d, want 124:\n%s", code, c.stderr.String())
	}
	if !strings.Contains(c.stderr.String(), "command exceeded the 1s hold cap") {
		t.Fatalf("no hold cap message:\n%s", c.stderr.String())
	}

	// Without the cap firing, the command's exit code passes through.
	c = e.start(e.dir, nil, "xcode", "sh", "-c", "exit 5")
	if code := c.wait(t); code != 5 {
		t.Fatalf("exit %d, want 5:\n%s", code, c.stderr.String())
	}
}

// withoutTimeout gives the script a PATH with the tools it needs and no `timeout` or
// `gtimeout`, which is a stock Mac and selects the shell watchdog.
func (e *lockEnv) withoutTimeout() {
	e.t.Helper()
	bin := filepath.Join(e.dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		e.t.Fatal(err)
	}
	for _, tool := range []string{"bash", "env", "sh", "sleep", "cat", "mkdir", "rm", "ls", "sort", "date", "pkill", "head", "awk", "touch", "true", "python3", "nice", "mv"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			e.t.Fatalf("tool %s needed for the PATH without timeout: %v", tool, err)
		}
		if err := os.Symlink(path, filepath.Join(bin, tool)); err != nil {
			e.t.Fatal(err)
		}
	}
	e.extra = []string{"PATH=" + bin}
}

// limiterPaths runs a test body once with `timeout` on PATH and once without it.
func limiterPaths(t *testing.T, body func(t *testing.T, e *lockEnv)) {
	t.Helper()
	for _, withTimeout := range []bool{true, false} {
		name := "timeout"
		if !withTimeout {
			name = "watchdog"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newLockEnv(t)
			if !withTimeout {
				e.withoutTimeout()
			}
			body(t, e)
		})
	}
}

func readPid(t *testing.T, path string) int {
	t.Helper()
	var pid int
	eventually(t, "pid file "+path, func() bool {
		b, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		n, err := strconv.Atoi(strings.TrimSpace(string(b)))
		pid = n
		return err == nil
	})
	return pid
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

// TestXcodeLockTermWaitsForCommandBeforeRelease sends SIGTERM to the wrapper while it holds
// the lock. The command must receive it, and the lock must stay until the command has
// exited. On the watchdog path, a later holder must then exit 0, with no stale cap mark.
func TestXcodeLockTermWaitsForCommandBeforeRelease(t *testing.T) {
	limiterPaths(t, func(t *testing.T, e *lockEnv) {
		pidFile := filepath.Join(e.dir, "cmd.pid")
		marker := filepath.Join(e.dir, "lock-state-at-exit")
		// On TERM the command winds down for 0.4 s and records whether the lock dir is
		// still there when it finishes.
		script := `echo $$ > "$1"; trap 'sleep 0.4; if [ -d "$2" ]; then echo held > "$3"; else echo released > "$3"; fi; exit 0' TERM; while :; do sleep 0.05; done`
		holder := e.start(e.dir, []string{"MONACO_LOCK_HOLD=2"}, "xcode", "sh", "-c", script, "sh", pidFile, e.xcodeLock(), marker)
		cmdPid := readPid(t, pidFile)
		eventually(t, "the lock to be held", func() bool { return exists(filepath.Join(e.xcodeLock(), "pid")) })

		if err := holder.cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		if code := holder.wait(t); code != 143 {
			t.Fatalf("wrapper exit %d, want 143:\n%s", code, holder.stderr.String())
		}
		got, err := os.ReadFile(marker)
		if err != nil {
			t.Fatalf("the command never saw the signal: %v\n%s", err, holder.stderr.String())
		}
		if strings.TrimSpace(string(got)) != "held" {
			t.Fatalf("lock was %s while the command was still exiting", strings.TrimSpace(string(got)))
		}
		if alive(cmdPid) {
			t.Fatalf("command pid %d still alive after the wrapper exited", cmdPid)
		}
		if exists(e.xcodeLock()) || countTickets(e.xcodeLock()+".queue") != 0 {
			t.Fatalf("lock or ticket left behind")
		}

		// The next holder outlives the first holder's 2 s cap. A leftover watchdog would
		// mark it capped and make it exit 124.
		next := e.start(e.dir, []string{"MONACO_LOCK_HOLD=60"}, "xcode", "sleep", "2.5")
		if code := next.wait(t); code != 0 {
			t.Fatalf("next holder exit %d, want 0:\n%s", code, next.stderr.String())
		}
		if strings.Contains(next.stderr.String(), "hold cap") {
			t.Fatalf("next holder blamed on a stale cap:\n%s", next.stderr.String())
		}
	})
}

// TestXcodeLockGroupInterruptStopsCommand sends SIGINT to the wrapper's whole process group,
// which is what Ctrl-C at a terminal does. GNU timeout puts the command in another group, so
// the wrapper has to forward the signal.
func TestXcodeLockGroupInterruptStopsCommand(t *testing.T) {
	limiterPaths(t, func(t *testing.T, e *lockEnv) {
		e.pgroup = true
		pidFile := filepath.Join(e.dir, "cmd.pid")
		holder := e.start(e.dir, nil, "xcode", "sh", "-c", `echo $$ > "$1"; exec sleep 30`, "sh", pidFile)
		cmdPid := readPid(t, pidFile)
		eventually(t, "the lock to be held", func() bool { return exists(filepath.Join(e.xcodeLock(), "pid")) })

		if err := syscall.Kill(-holder.cmd.Process.Pid, syscall.SIGINT); err != nil {
			t.Fatal(err)
		}
		// The command must stop at once, not when sleep 30 ends.
		eventually(t, "the command to stop", func() bool { return !alive(cmdPid) })
		if code := holder.wait(t); code != 130 {
			t.Fatalf("wrapper exit %d, want 130:\n%s", code, holder.stderr.String())
		}
		if exists(e.xcodeLock()) {
			t.Fatalf("lock dir still present")
		}
	})
}

func TestXcodeLockRecordsTheWaitOnceItTakesTheLock(t *testing.T) {
	t.Parallel()
	e := newLockEnv(t)
	started := filepath.Join(e.dir, "holder.started")
	release := filepath.Join(e.dir, "holder.release")
	waited := filepath.Join(e.dir, "waited")
	ran := filepath.Join(e.dir, "ran")

	holder := e.start(e.dir, nil, append([]string{"xcode"}, holdUntil(started, release)...)...)
	eventually(t, "the holder to hold the lock", func() bool { return exists(started) })
	w := e.start(e.dir, []string{"MONACO_LOCK_WAITED=" + waited}, "xcode", "sh", "-c", `[ -s "$1" ] && touch "$2"`, "sh", waited, ran)
	eventually(t, "the waiter to queue", func() bool { return countTickets(e.xcodeLock()+".queue") == 1 })
	time.Sleep(1100 * time.Millisecond)
	if exists(waited) {
		t.Fatalf("the wait was recorded before the lock was taken")
	}
	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := holder.wait(t); code != 0 {
		t.Fatalf("holder exit %d:\n%s", code, holder.stderr.String())
	}
	if code := w.wait(t); code != 0 || !exists(ran) {
		t.Fatalf("waiter exit %d, ran before the wait was written: %v:\n%s", code, exists(ran), w.stderr.String())
	}
	got, err := os.ReadFile(waited)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := strconv.Atoi(strings.TrimSpace(string(got))); err != nil || n < 1 {
		t.Fatalf("waited %q, want at least 1 whole second", got)
	}

	free := e.start(e.dir, []string{"MONACO_LOCK_WAITED=" + waited}, "xcode", "true")
	if code := free.wait(t); code != 0 {
		t.Fatalf("free lock exit %d:\n%s", code, free.stderr.String())
	}
	got, _ = os.ReadFile(waited)
	if lines := strings.Fields(string(got)); len(lines) != 2 || lines[1] != "0" {
		t.Fatalf("a free lock appends 0 after the first wait, got %q", got)
	}
}

// With two slots, a second holder runs while the first still holds its slot, and a third
// caller waits until one of them leaves, naming both holders while it waits.
//
// Each caller starts only once the one before it holds a slot, so it is alone in the queue at
// position 1 and its first pass decides the outcome: a free slot runs it at once, and none
// logs "queue position 1" at once. Every wait below ends on one of those two events, whatever
// the runner's speed.
func TestXcodeLockSlotsRunHoldersTogether(t *testing.T) {
	t.Parallel()
	e := newLockEnv(t)
	two := []string{"MONACO_XCODE_SLOTS=2"}
	aIn, bIn, cIn := filepath.Join(e.dir, "a.in"), filepath.Join(e.dir, "b.in"), filepath.Join(e.dir, "c.in")
	aRelease, bRelease := filepath.Join(e.dir, "a.release"), filepath.Join(e.dir, "b.release")
	queued := func(c *call) bool { return strings.Contains(c.stderr.String(), "queue position 1 behind ") }

	a := e.start(e.dir, two, append([]string{"xcode"}, holdUntil(aIn, aRelease)...)...)
	await(t, "the first holder to run", func() bool { return exists(aIn) }, a)

	b := e.start(e.dir, two, append([]string{"xcode"}, holdUntil(bIn, bRelease)...)...)
	// Cleanups run last-registered first, so on a failure this releases the holders before
	// start's cleanup kills the wrappers; a held sh keeps stderr open and Wait would block.
	t.Cleanup(func() {
		_ = os.WriteFile(aRelease, nil, 0o644)
		_ = os.WriteFile(bRelease, nil, 0o644)
	})
	await(t, "the second holder to run or queue", func() bool { return exists(bIn) || queued(b) }, a, b)
	if !exists(bIn) {
		t.Fatalf("second holder queued while a slot was free:\n%s", b.stderr.String())
	}
	if !exists(e.xcodeLock()) || !exists(e.xcodeLock()+".2") {
		t.Fatalf("holders are not in slots 1 and 2")
	}
	aPid := readPid(t, filepath.Join(e.xcodeLock(), "pid"))
	bPid := readPid(t, filepath.Join(e.xcodeLock()+".2", "pid"))

	c := e.start(e.dir, two, "xcode", "touch", cIn)
	await(t, "the third caller to queue", func() bool { return queued(c) }, a, b, c)
	log := c.stderr.String()
	if !strings.Contains(log, "pid "+strconv.Itoa(aPid)+" (") || !strings.Contains(log, "pid "+strconv.Itoa(bPid)+" (") {
		t.Fatalf("waiter log does not name both holders %d and %d:\n%s", aPid, bPid, log)
	}
	time.Sleep(500 * time.Millisecond)
	if exists(cIn) {
		t.Fatalf("third caller ran while both slots were held")
	}

	if err := os.WriteFile(bRelease, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := b.wait(t); code != 0 {
		t.Fatalf("holder b exit %d:\n%s", code, b.stderr.String())
	}
	if code := c.wait(t); code != 0 || !exists(cIn) {
		t.Fatalf("third caller exit %d, ran %v:\n%s", code, exists(cIn), c.stderr.String())
	}
	if err := os.WriteFile(aRelease, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := a.wait(t); code != 0 {
		t.Fatalf("holder a exit %d:\n%s", code, a.stderr.String())
	}
	if exists(e.xcodeLock()) || exists(e.xcodeLock()+".2") {
		t.Fatalf("a slot dir is still present after every caller exited")
	}
}

// Two builds into one derived data path fail with "database is locked", so a second caller
// on the same -derivedDataPath waits even while a slot is free, and a caller on another
// path still runs beside the first.
func TestXcodeLockSameDerivedDataSerializes(t *testing.T) {
	t.Parallel()
	e := newLockEnv(t)
	two := []string{"MONACO_XCODE_SLOTS=2"}
	derived := filepath.Join(e.dir, "checkout", ".build", "DerivedData")
	other := filepath.Join(e.dir, "other", ".build", "DerivedData")
	aIn, bIn, cIn := filepath.Join(e.dir, "a.in"), filepath.Join(e.dir, "b.in"), filepath.Join(e.dir, "c.in")
	aRelease, cRelease := filepath.Join(e.dir, "a.release"), filepath.Join(e.dir, "c.release")

	a := e.start(e.dir, two, append(append([]string{"xcode"}, holdUntil(aIn, aRelease)...), "-derivedDataPath", derived)...)
	eventually(t, "the first build to run", func() bool { return exists(aIn) })
	aPid := readPid(t, derived+".lock/pid")

	b := e.start(e.dir, two, "xcode", "sh", "-c", `touch "$1"`, "sh", bIn, "-derivedDataPath", derived+"/")
	c := e.start(e.dir, two, append(append([]string{"xcode"}, holdUntil(cIn, cRelease)...), "-derivedDataPath", other)...)
	eventually(t, "the build on another path to run", func() bool { return exists(cIn) })
	eventually(t, "the second build on the same path to wait", func() bool {
		return strings.Contains(b.stderr.String(), "waiting for "+derived+" behind pid "+strconv.Itoa(aPid)+" (")
	})
	time.Sleep(300 * time.Millisecond)
	if exists(bIn) {
		t.Fatalf("second build ran while the first held %s", derived)
	}

	if err := os.WriteFile(aRelease, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := a.wait(t); code != 0 {
		t.Fatalf("first build exit %d:\n%s", code, a.stderr.String())
	}
	if code := b.wait(t); code != 0 || !exists(bIn) {
		t.Fatalf("second build exit %d, ran %v:\n%s", code, exists(bIn), b.stderr.String())
	}
	if err := os.WriteFile(cRelease, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if code := c.wait(t); code != 0 {
		t.Fatalf("other-path build exit %d:\n%s", code, c.stderr.String())
	}
	if exists(derived+".lock") || exists(other+".lock") {
		t.Fatalf("a derived data lock is still present after every caller exited")
	}
}

// An xcodebuild left running by a killed wrapper holds no lock dir, yet it still owns the
// build database. The next caller on its path waits for it to exit.
func TestXcodeLockWaitsForOrphanXcodebuild(t *testing.T) {
	t.Parallel()
	e := newLockEnv(t)
	derived := filepath.Join(e.dir, "checkout", ".build", "DerivedData")
	release := filepath.Join(e.dir, "orphan.release")
	ran := filepath.Join(e.dir, "ran")

	fake := filepath.Join(e.dir, "bin", "xcodebuild")
	writeExecutable(t, fake, "#!/bin/sh\nwhile [ ! -e \"$ORPHAN_RELEASE\" ]; do sleep 0.05; done\n")
	orphan := exec.Command(fake, "-project", "Monaco.xcodeproj", "-derivedDataPath", derived, "build")
	orphan.Env = append(os.Environ(), "ORPHAN_RELEASE="+release)
	if err := orphan.Start(); err != nil {
		t.Fatal(err)
	}
	orphanDone := make(chan error, 1)
	go func() { orphanDone <- orphan.Wait() }()
	t.Cleanup(func() {
		_ = orphan.Process.Kill()
		<-orphanDone
	})

	w := e.start(e.dir, nil, "xcode", "sh", "-c", `touch "$1"`, "sh", ran, "-derivedDataPath", derived)
	eventually(t, "the caller to wait for the orphan", func() bool {
		return strings.Contains(w.stderr.String(), "behind pid "+strconv.Itoa(orphan.Process.Pid)+" (an xcodebuild outside this lock)")
	})
	time.Sleep(300 * time.Millisecond)
	if exists(ran) {
		t.Fatalf("caller ran while an orphaned xcodebuild still built into %s", derived)
	}

	if err := os.WriteFile(release, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := <-orphanDone; err != nil {
		t.Fatalf("orphan: %v", err)
	}
	orphanDone <- nil
	if code := w.wait(t); code != 0 || !exists(ran) {
		t.Fatalf("caller exit %d, ran %v:\n%s", code, exists(ran), w.stderr.String())
	}
}

// With MONACO_XCODE_SLOTS unset, the clone's .git/.monaco/xcode-slots sets the slot count
// for every worktree of the machine.
func TestXcodeLockReadsTheCloneSlotsFile(t *testing.T) {
	t.Parallel()
	e := newLockEnv(t)
	repo := filepath.Join(e.dir, "repo")
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".git", ".monaco"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", ".monaco", "xcode-slots"), []byte("2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	unset := []string{"MONACO_XCODE_SLOTS="}
	aIn, bIn := filepath.Join(e.dir, "a.in"), filepath.Join(e.dir, "b.in")
	aRelease, bRelease := filepath.Join(e.dir, "a.release"), filepath.Join(e.dir, "b.release")
	t.Cleanup(func() {
		_ = os.WriteFile(aRelease, nil, 0o644)
		_ = os.WriteFile(bRelease, nil, 0o644)
	})

	a := e.start(repo, unset, append([]string{"xcode"}, holdUntil(aIn, aRelease)...)...)
	await(t, "the first holder to run", func() bool { return exists(aIn) }, a)
	b := e.start(repo, unset, append([]string{"xcode"}, holdUntil(bIn, bRelease)...)...)
	await(t, "the second holder to run", func() bool { return exists(bIn) }, a, b)

	for _, c := range []struct {
		release string
		call    *call
	}{{aRelease, a}, {bRelease, b}} {
		if err := os.WriteFile(c.release, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if code := c.call.wait(t); code != 0 {
			t.Fatalf("holder exit %d:\n%s", code, c.call.stderr.String())
		}
	}
}

// A caller already queued rereads xcode-slots on every poll, so raising the count lets it
// take the new slot beside the holder instead of waiting for the holder to finish.
func TestXcodeLockTakesASlotAddedWhileQueued(t *testing.T) {
	t.Parallel()
	e := newLockEnv(t)
	repo := filepath.Join(e.dir, "repo")
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	slotsFile := filepath.Join(repo, ".git", ".monaco", "xcode-slots")
	if err := os.MkdirAll(filepath.Dir(slotsFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(slotsFile, []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	unset := []string{"MONACO_XCODE_SLOTS="}
	aIn, bIn := filepath.Join(e.dir, "a.in"), filepath.Join(e.dir, "b.in")
	aRelease, bRelease := filepath.Join(e.dir, "a.release"), filepath.Join(e.dir, "b.release")

	a := e.start(repo, unset, append([]string{"xcode"}, holdUntil(aIn, aRelease)...)...)
	await(t, "the first holder to run", func() bool { return exists(aIn) }, a)
	b := e.start(repo, append(unset, "MONACO_XCODE_LOCK_TIMEOUT=10"),
		append([]string{"xcode"}, holdUntil(bIn, bRelease)...)...)
	// Registered after both starts so it runs before their kills: a held sh keeps stderr
	// open and Wait would block.
	t.Cleanup(func() {
		_ = os.WriteFile(aRelease, nil, 0o644)
		_ = os.WriteFile(bRelease, nil, 0o644)
	})
	await(t, "the second caller to queue", func() bool {
		return strings.Contains(b.stderr.String(), "queue position 1 behind ")
	}, a, b)
	if exists(bIn) {
		t.Fatalf("second caller ran with one slot held")
	}

	if err := os.WriteFile(slotsFile, []byte("2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	await(t, "the queued caller to take the new slot", func() bool { return exists(bIn) }, a, b)
	if !exists(e.xcodeLock() + ".2") {
		t.Fatalf("the queued caller is not in slot 2")
	}
	for _, c := range []struct {
		release string
		call    *call
	}{{bRelease, b}, {aRelease, a}} {
		if err := os.WriteFile(c.release, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if code := c.call.wait(t); code != 0 {
			t.Fatalf("holder exit %d:\n%s", code, c.call.stderr.String())
		}
	}
}

// laneEnv names a caller's lane and gives the class two slots.
func laneEnv(lane string) []string {
	return []string{"MONACO_LANE=" + lane, "MONACO_XCODE_SLOTS=2"}
}

func release(t *testing.T, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// Two slots are free, yet two callers of one lane run one after the other.
func TestXcodeLockLaneHoldsOneSlot(t *testing.T) {
	t.Parallel()
	e := newLockEnv(t)
	aIn, bIn := filepath.Join(e.dir, "a.in"), filepath.Join(e.dir, "b.in")
	aRel, bRel := filepath.Join(e.dir, "a.rel"), filepath.Join(e.dir, "b.rel")

	a := e.start(e.dir, laneEnv("a"), append([]string{"xcode"}, holdUntil(aIn, aRel)...)...)
	await(t, "a to run", func() bool { return exists(aIn) }, a)
	b := e.start(e.dir, laneEnv("a"), append([]string{"xcode"}, holdUntil(bIn, bRel)...)...)
	// Registered after the starts, so it runs before their kills; a held sh keeps stderr open.
	t.Cleanup(func() { release(t, aRel, bRel) })
	await(t, "b to queue", func() bool { return strings.Contains(b.stderr.String(), "queue position ") }, a, b)
	time.Sleep(500 * time.Millisecond)
	if exists(bIn) || exists(e.xcodeLock()+".2") {
		t.Fatalf("a second caller of lane a ran beside the first")
	}
	release(t, aRel)
	if code := a.wait(t); code != 0 {
		t.Fatalf("a exit %d", code)
	}
	await(t, "b to run after a", func() bool { return exists(bIn) }, b)
	release(t, bRel)
	if code := b.wait(t); code != 0 {
		t.Fatalf("b exit %d:\n%s", code, b.stderr.String())
	}
}

// A lane-blocked waiter at the head of the queue does not keep another lane from a free slot.
func TestXcodeLockBlockedLaneDoesNotBlockTheQueue(t *testing.T) {
	t.Parallel()
	e := newLockEnv(t)
	aIn, a2In, bIn := filepath.Join(e.dir, "a.in"), filepath.Join(e.dir, "a2.in"), filepath.Join(e.dir, "b.in")
	aRel, a2Rel := filepath.Join(e.dir, "a.rel"), filepath.Join(e.dir, "a2.rel")

	a := e.start(e.dir, laneEnv("a"), append([]string{"xcode"}, holdUntil(aIn, aRel)...)...)
	await(t, "a to run", func() bool { return exists(aIn) }, a)
	a2 := e.start(e.dir, laneEnv("a"), append([]string{"xcode"}, holdUntil(a2In, a2Rel)...)...)
	t.Cleanup(func() { release(t, aRel, a2Rel) })
	await(t, "a2 to queue", func() bool { return strings.Contains(a2.stderr.String(), "queue position ") }, a, a2)

	b := e.start(e.dir, laneEnv("b"), "xcode", "touch", bIn)
	await(t, "b to run behind the blocked head", func() bool { return exists(bIn) }, a, a2, b)
	if code := b.wait(t); code != 0 {
		t.Fatalf("b exit %d:\n%s", code, b.stderr.String())
	}
	if exists(a2In) {
		t.Fatalf("a2 ran while lane a held a slot")
	}
}

// Different lanes run together up to the slot count.
func TestXcodeLockDifferentLanesRunTogether(t *testing.T) {
	t.Parallel()
	e := newLockEnv(t)
	aIn, bIn := filepath.Join(e.dir, "a.in"), filepath.Join(e.dir, "b.in")
	aRel, bRel := filepath.Join(e.dir, "a.rel"), filepath.Join(e.dir, "b.rel")

	a := e.start(e.dir, laneEnv("a"), append([]string{"xcode"}, holdUntil(aIn, aRel)...)...)
	b := e.start(e.dir, laneEnv("b"), append([]string{"xcode"}, holdUntil(bIn, bRel)...)...)
	t.Cleanup(func() { release(t, aRel, bRel) })
	await(t, "both lanes to run", func() bool { return exists(aIn) && exists(bIn) }, a, b)
	release(t, aRel, bRel)
	if a.wait(t) != 0 || b.wait(t) != 0 {
		t.Fatalf("a lane exited non-zero")
	}
}

// A wrapped script that calls the wrapper again runs at once instead of queuing behind itself.
func TestXcodeLockNestedCallRunsAtOnce(t *testing.T) {
	t.Parallel()
	e := newLockEnv(t)
	out := filepath.Join(e.dir, "nested.out")
	c := e.start(e.dir, laneEnv("a"), "xcode", "sh", "-c", `"$1" xcode touch "$2"`, "sh", e.script, out)
	if code := c.wait(t); code != 0 || !exists(out) {
		t.Fatalf("nested call exit %d, ran %v:\n%s", code, exists(out), c.stderr.String())
	}
}

// Each xcode run appends a row with its lane, HEAD and exit code, and the wrapper exits with
// the command's code.
func TestXcodeLockLogsEachBuild(t *testing.T) {
	t.Parallel()
	e := newLockEnv(t)
	repo := filepath.Join(e.dir, "repo")
	for _, args := range [][]string{
		{"init", "-q", repo},
		{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "x"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	head, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	ok := e.start(repo, laneEnv("lg"), "xcode", "true", "test")
	if code := ok.wait(t); code != 0 {
		t.Fatalf("exit %d", code)
	}
	bad := e.start(repo, laneEnv("lg"), "xcode", "sh", "-c", "exit 7")
	if code := bad.wait(t); code != 7 {
		t.Fatalf("wrapper exit %d, want 7", code)
	}
	raw, err := os.ReadFile(filepath.Join(repo, ".git", ".monaco", "xcode-builds.tsv"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "start\tlane\t") {
		t.Fatalf("log is not header plus two rows:\n%s", raw)
	}
	f := strings.Split(lines[1], "\t")
	if len(f) != 9 || f[1] != "lg" || f[3] != strings.TrimSpace(string(head)) || f[5] != "test" || f[7] != "0" {
		t.Fatalf("first row wrong: %q", lines[1])
	}
	f = strings.Split(lines[2], "\t")
	if len(f) != 9 || f[5] != "other" || f[7] != "7" {
		t.Fatalf("second row wrong: %q", lines[2])
	}
}
