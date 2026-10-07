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

const slotPoll = 2 * time.Second

var errTicketLost = errors.New("the stage 0 ticket was removed while waiting")

type checkQueue struct {
	dir   string
	slots func() int
	alive func(pid int) bool
	after func(time.Duration) <-chan time.Time
	now   func() time.Time
}

func (env *Env) checkQueue() *checkQueue {
	return &checkQueue{
		dir: filepath.Join(env.Common, ".monaco", "check-queue"), slots: env.slots,
		alive: pidAlive, after: env.After, now: env.Now,
	}
}

func (env *Env) slots() int {
	cfg, _, err := readConfig(env.Work, env.Common)
	if err != nil {
		return env.Config.Slots
	}
	return cfg.Slots
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

func (q *checkQueue) drop(name string) {
	_ = os.Remove(filepath.Join(q.dir, name))
}

func (q *checkQueue) live() ([]string, error) {
	entries, err := os.ReadDir(q.dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read the stage 0 queue: %w", err)
	}
	var live []string
	for _, e := range entries {
		_, pidText, _ := strings.Cut(e.Name(), "-")
		if pid, err := strconv.Atoi(pidText); err != nil || !q.alive(pid) {
			q.drop(e.Name())
			continue
		}
		live = append(live, e.Name())
	}
	slices.Sort(live)
	return live, nil
}

func (q *checkQueue) standing(name string) (position, total int, err error) {
	live, err := q.live()
	if err != nil {
		return 0, 0, err
	}
	at := slices.Index(live, name)
	if at < 0 {
		return 0, 0, errTicketLost
	}
	return at + 1, len(live), nil
}

func (q *checkQueue) await(ctx context.Context, name string, stdout io.Writer) error {
	last := 0
	for {
		position, total, err := q.standing(name)
		if err != nil {
			return err
		}
		if position <= q.slots() {
			return nil
		}
		if position != last {
			_, _ = fmt.Fprintf(stdout, "waiting for a stage 0 slot: position %d of %d\n", position, total)
			last = position
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for a stage 0 slot: %w", ctx.Err())
		case <-q.after(slotPoll):
		}
	}
}

func (env *Env) takeSlot(ctx context.Context, stdout io.Writer) (func(), error) {
	q := env.checkQueue()
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
