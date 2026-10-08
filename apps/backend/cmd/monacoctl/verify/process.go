package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"syscall"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

var (
	errExitedBeforeStop = errors.New("exited before it was stopped")
	errDiedOnSIGTERM    = errors.New("died on SIGTERM instead of handling it")
)

type Line struct {
	Process string
	Text    string
	Stderr  bool
}

type Logs struct {
	mu      sync.Mutex
	lines   []Line
	changed chan struct{}
}

func (l *Logs) add(process, text string) {
	l.append(Line{Process: process, Text: text})
}

func (l *Logs) append(line Line) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, line)
	if l.changed != nil {
		close(l.changed)
		l.changed = nil
	}
}

func (l *Logs) Lines() []Line {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Line(nil), l.lines...)
}

func (l *Logs) mark() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.lines)
}

func (l *Logs) since(from int) ([]string, <-chan struct{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	texts := make([]string, 0, len(l.lines)-from)
	for _, line := range l.lines[from:] {
		texts = append(texts, line.Text)
	}
	if l.changed == nil {
		l.changed = make(chan struct{})
	}
	return texts, l.changed
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

func (l *Logs) lastPanicOrLine(process string) string {
	lines := l.Lines()
	for i := len(lines) - 1; i >= 0; i-- {
		if line := lines[i]; line.Process == process && line.Stderr && strings.HasPrefix(line.Text, "panic:") {
			return line.Text
		}
	}
	return l.tail(process, 1)
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
	p.cmd.Stdout = &lineWriter{line: func(text string) { p.line(Line{Process: name, Text: text}) }}
	p.cmd.Stderr = &lineWriter{line: func(text string) { p.line(Line{Process: name, Text: text, Stderr: true}) }}
	if err := p.cmd.Start(); err != nil {
		close(p.exited)
		return p, errs.Wrap(err, errs.CodeInternal, op, slog.String("process", name))
	}
	go func() {
		p.err = p.cmd.Wait()
		close(p.exited)
	}()
	return p, p.awaitListening(ctx)
}

func (p *process) awaitListening(ctx context.Context) error {
	select {
	case p.addr = <-p.listening:
		return nil
	case <-p.exited:
		select {
		case p.addr = <-p.listening:
			return nil
		default:
		}
		return fmt.Errorf("%s exited before boot.listening: %w\n%s", p.name, p.err, p.logs.tail(p.name, 20))
	case <-ctx.Done():
		return fmt.Errorf("%s never logged boot.listening: %w", p.name, context.Cause(ctx))
	}
}

func (p *process) line(line Line) {
	p.logs.append(line)
	var boot struct {
		Msg  string `json:"msg"`
		Addr string `json:"addr"`
	}
	if json.Unmarshal([]byte(line.Text), &boot) == nil && boot.Msg == "boot.listening" {
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
		return p.exitError(errExitedBeforeStop)
	}
	if ctx.Err() != nil {
		p.kill()
		return fmt.Errorf("%s was killed without a graceful stop because the budget was already spent: %w",
			p.name, context.Cause(ctx))
	}
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-p.exited:
		return p.diedOnSIGTERM()
	case <-ctx.Done():
		p.kill()
		return fmt.Errorf("%s did not exit within the budget after SIGTERM and was killed: %w",
			p.name, context.Cause(ctx))
	}
}

func (p *process) diedOnSIGTERM() error {
	status, ok := p.cmd.ProcessState.Sys().(syscall.WaitStatus)
	if ok && status.Signaled() && status.Signal() == syscall.SIGTERM {
		return p.exitError(errDiedOnSIGTERM)
	}
	return nil
}

func (p *process) exitError(reason error) error {
	if p.addr == "" {
		return nil
	}
	last := p.logs.tail(p.name, 1)
	if !p.cmd.ProcessState.Success() {
		last = p.logs.lastPanicOrLine(p.name)
	}
	return fmt.Errorf("%s %w (%s), last line: %s", p.name, reason, p.cmd.ProcessState, last)
}

func (p *process) kill() {
	_ = p.cmd.Process.Kill()
	<-p.exited
}
