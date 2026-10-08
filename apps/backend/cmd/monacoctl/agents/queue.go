package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	slotPoll     = 2 * time.Second
	admittedLine = "run"
)

func checkClasses() []string { return []string{"db", "cpu"} }

var errTicketLost = errors.New("the stage 0 ticket was removed while waiting")

type checkQueue struct {
	class string
	dir   string
	slots func() int
	alive func(pid int) bool
	after func(time.Duration) <-chan time.Time
	now   func() time.Time
}

func (env *Env) checkQueue(class string) *checkQueue {
	return &checkQueue{
		class: class, dir: filepath.Join(env.Common, ".monaco", "check-queue", class),
		slots: func() int { return env.tokens(class) }, alive: pidAlive, after: env.After, now: env.Now,
	}
}

func (env *Env) tokens(class string) int {
	cfg, _, err := readConfig(env.Work, env.Common)
	if err != nil {
		cfg = env.Config
	}
	return cfg.Tokens.of(class)
}

func pidAlive(pid int) bool {
	p, _ := os.FindProcess(pid)
	err := p.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

func (q *checkQueue) take(worktree string, pid int) (string, error) {
	name := fmt.Sprintf("%d-%d", q.now().UnixNano(), pid)
	err := os.MkdirAll(q.dir, 0o750)
	if err == nil {
		err = os.WriteFile(filepath.Join(q.dir, name), []byte(worktree+"\n"), 0o600)
	}
	if err != nil {
		return "", fmt.Errorf("take a stage 0 ticket: %w", err)
	}
	return name, nil
}

func (q *checkQueue) admit(name string) error {
	f, err := os.OpenFile(filepath.Join(q.dir, name), os.O_APPEND|os.O_WRONLY, 0o600)
	if err == nil {
		_, err = f.WriteString(admittedLine + "\n")
		err = errors.Join(err, f.Close())
	}
	if err != nil {
		return fmt.Errorf("record the stage 0 admission: %w", err)
	}
	return nil
}

func (q *checkQueue) drop(name string) {
	_ = os.Remove(filepath.Join(q.dir, name))
}

type queueStanding struct {
	position, total int
	run             bool
	laneBusy        bool
}

type queueTicket struct {
	name, lane string
	admitted   bool
}

func (q *checkQueue) live() ([]queueTicket, error) {
	entries, err := os.ReadDir(q.dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read the stage 0 queue: %w", err)
	}
	var live []queueTicket
	for _, e := range entries {
		_, pidText, _ := strings.Cut(e.Name(), "-")
		if pid, err := strconv.Atoi(pidText); err != nil || !q.alive(pid) {
			q.drop(e.Name())
			continue
		}
		content, err := os.ReadFile(filepath.Join(q.dir, e.Name()))
		if err != nil {
			continue
		}
		worktree, rest, _ := strings.Cut(string(content), "\n")
		live = append(live, queueTicket{
			name: e.Name(), lane: lane(worktree),
			admitted: strings.Contains("\n"+rest, "\n"+admittedLine+"\n"),
		})
	}
	slices.SortFunc(live, func(a, b queueTicket) int { return strings.Compare(a.name, b.name) })
	return live, nil
}

func (q *checkQueue) standing(name string) (queueStanding, error) {
	live, err := q.live()
	if err != nil {
		return queueStanding{}, err
	}
	at := slices.IndexFunc(live, func(t queueTicket) bool { return t.name == name })
	if at < 0 {
		return queueStanding{}, errTicketLost
	}
	got := queueStanding{position: at + 1, total: len(live)}
	got.run = live[at].admitted || slices.Contains(admissions(live, q.slots()), name)
	got.laneBusy = !got.run && slices.ContainsFunc(live, func(t queueTicket) bool {
		return t.admitted && t.lane == live[at].lane
	})
	return got, nil
}

func admissions(live []queueTicket, slots int) []string {
	running, busyLanes := 0, map[string]bool{}
	var waiting, admit []string
	lanes := map[string]string{}
	for _, t := range live {
		lanes[t.name] = t.lane
		if t.admitted {
			running++
			busyLanes[t.lane] = true
		} else {
			waiting = append(waiting, t.name)
		}
	}
	var blocked []string
	for _, name := range waiting {
		if running < slots && !busyLanes[lanes[name]] {
			running++
			busyLanes[lanes[name]] = true
			admit = append(admit, name)
		} else {
			blocked = append(blocked, name)
		}
	}
	for _, name := range blocked {
		if running < slots {
			running++
			admit = append(admit, name)
		}
	}
	return admit
}

func lane(worktree string) string {
	if filepath.Base(filepath.Dir(worktree)) != ".worktrees" {
		return worktree
	}
	base := filepath.Base(worktree)
	end := strings.IndexFunc(base, func(r rune) bool { return r < '0' || r > '9' })
	if end < 0 {
		end = len(base)
	}
	if end == 0 {
		return worktree
	}
	return base[:end]
}

func (q *checkQueue) await(ctx context.Context, name string, stdout io.Writer) error {
	last := ""
	for {
		got, err := q.standing(name)
		if err != nil {
			return err
		}
		if got.run {
			return q.admit(name)
		}
		line := fmt.Sprintf("waiting for a %s token: position %d of %d", q.class, got.position, got.total)
		if got.laneBusy {
			line += "; this lane already runs a check"
		}
		if line != last {
			_, _ = fmt.Fprintln(stdout, line)
			last = line
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for a %s token: %w", q.class, ctx.Err())
		case <-q.after(slotPoll):
		}
	}
}

func (env *Env) takeToken(ctx context.Context, class string, stdout io.Writer) (func(), error) {
	q := env.checkQueue(class)
	name, err := q.take(env.Work, os.Getpid())
	if err != nil {
		return nil, detailErr(errs.CodeInvalidInput, "monacoctl.agents.check", err.Error())
	}
	if err := q.await(ctx, name, stdout); err != nil {
		q.drop(name)
		return nil, err
	}
	return func() { q.drop(name) }, nil
}
