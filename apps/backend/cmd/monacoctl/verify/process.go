package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Line struct {
	Process string
	Text    string
}

type Logs struct {
	mu    sync.Mutex
	lines []Line
}

func (l *Logs) add(process, text string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, Line{Process: process, Text: text})
}

func (l *Logs) Lines() []Line {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Line(nil), l.lines...)
}

func (l *Logs) tail(process string, n int) string {
	var out []string
	for _, line := range l.Lines() {
		if line.Process == process {
			out = append(out, line.Text)
		}
	}
	return strings.Join(out[max(0, len(out)-n):], "\n")
}

type lineWriter struct {
	mu      sync.Mutex
	pending []byte
	line    func(string)
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, p...)
	for {
		i := bytes.IndexByte(w.pending, '\n')
		if i < 0 {
			return len(p), nil
		}
		w.line(string(w.pending[:i]))
		w.pending = w.pending[i+1:]
	}
}

type process struct {
	name      string
	cmd       *exec.Cmd
	addr      string
	logs      *Logs
	listening chan string
	exited    chan struct{}
	err       error
}

func startProcess(ctx context.Context, name, bin string, env []string, logs *Logs) (*process, error) {
	const op = "verify.startProcess"
	p := &process{
		name: name, cmd: exec.CommandContext(context.WithoutCancel(ctx), bin), logs: logs,
		listening: make(chan string, 1), exited: make(chan struct{}),
	}
	p.cmd.Env = env
	out := &lineWriter{line: p.line}
	p.cmd.Stdout, p.cmd.Stderr = out, out
	if err := p.cmd.Start(); err != nil {
		close(p.exited)
		return p, errs.Wrap(err, errs.CodeInternal, op, slog.String("process", name))
	}
	go func() {
		p.err = p.cmd.Wait()
		close(p.exited)
	}()
	select {
	case p.addr = <-p.listening:
		return p, nil
	case <-p.exited:
		return p, fmt.Errorf("%s exited before boot.listening: %w\n%s", name, p.err, logs.tail(name, 20))
	case <-ctx.Done():
		return p, fmt.Errorf("%s never logged boot.listening: %w", name, context.Cause(ctx))
	}
}

func (p *process) line(text string) {
	p.logs.add(p.name, text)
	var boot struct {
		Msg  string `json:"msg"`
		Addr string `json:"addr"`
	}
	if json.Unmarshal([]byte(text), &boot) == nil && boot.Msg == "boot.listening" {
		select {
		case p.listening <- boot.Addr:
		default:
		}
	}
}

func (p *process) running() bool {
	select {
	case <-p.exited:
		return false
	default:
		return true
	}
}

func (p *process) stop(ctx context.Context) error {
	if !p.running() {
		return nil
	}
	if ctx.Err() != nil {
		p.kill()
		return fmt.Errorf("%s was killed without a graceful stop because the budget was already spent: %w",
			p.name, context.Cause(ctx))
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.exited:
		return nil
	case <-ctx.Done():
		p.kill()
		return fmt.Errorf("%s did not exit within the budget after SIGTERM and was killed: %w",
			p.name, context.Cause(ctx))
	}
}

func (p *process) kill() {
	_ = p.cmd.Process.Kill()
	<-p.exited
}
