package testkit

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"testing"
)

const runMainEnv = "TESTKIT_RUN_MAIN"

func childMain(main func()) {
	if os.Getenv(runMainEnv) == "1" {
		main()
		os.Exit(0)
	}
}

func MainCommand(t *testing.T, env []string, args ...string) *exec.Cmd {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), exe, args...)
	cmd.Env = append(slices.Clone(env), runMainEnv+"=1")
	if dir := flag.Lookup("test.gocoverdir"); dir != nil && dir.Value.String() != "" {
		cmd.Env = append(cmd.Env, "GOCOVERDIR="+dir.Value.String())
	}
	return cmd
}

type MainProcess struct {
	Addr   string
	cmd    *exec.Cmd
	lines  *bufio.Scanner
	stderr strings.Builder
}

func StartMain(t *testing.T, env []string) *MainProcess {
	t.Helper()
	p := &MainProcess{cmd: MainCommand(t, env)}
	pipe, err := p.cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p.lines = bufio.NewScanner(pipe)
	for p.lines.Scan() {
		p.stderr.WriteString(p.lines.Text() + "\n")
		var line struct {
			Msg  string `json:"msg"`
			Addr string `json:"addr"`
		}
		if json.Unmarshal(p.lines.Bytes(), &line) == nil && line.Msg == "boot.listening" {
			p.Addr = line.Addr
			return p
		}
	}
	t.Fatalf("main exited before boot.listening: %v\n%s", p.cmd.Wait(), p.stderr.String())
	return nil
}

func (p *MainProcess) Terminate() (string, error) {
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return p.stderr.String(), fmt.Errorf("signal: %w", err)
	}
	for p.lines.Scan() {
		p.stderr.WriteString(p.lines.Text() + "\n")
	}
	if err := p.cmd.Wait(); err != nil {
		return p.stderr.String(), fmt.Errorf("wait: %w", err)
	}
	return p.stderr.String(), nil
}

func Get(t *testing.T, url string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(body)
}
