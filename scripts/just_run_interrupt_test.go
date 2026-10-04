package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A real binary, not a script: `just stop backend` matches `^<root>/bin/(api|worker)$`, and a
// script's command line starts with its interpreter instead.
const sleeperSource = `package main

import (
	"os"
	"os/signal"
	"syscall"
)

func main() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, os.Interrupt)
	<-c
}
`

const sleeperGo = `#!/usr/bin/env bash
out="$PWD/../../bin"
mkdir -p "$out"
for name in api worker monacoctl; do cp "$SLEEPER" "$out/$name"; done
`

// iosSimStub stands in for the mobile build: it waits for the backend to come up, then
// either keeps building (MOBILE_EXIT unset) or fails with MOBILE_EXIT.
const iosSimStub = `#!/usr/bin/env bash
until pgrep -f "^$PWD/bin/worker$" >/dev/null; do sleep 0.05; done
touch "$MOBILE_STARTED"
if [[ -n "${MOBILE_EXIT:-}" ]]; then exit "$MOBILE_EXIT"; fi
while :; do sleep 0.1; done
`

type justRun struct {
	cmd     *exec.Cmd
	done    chan error
	backend string // pgrep pattern for this sandbox's api and worker
	started string
	output  *strings.Builder
}

// startJustRun runs `just run` in its own process group, with api and worker as real
// binaries and the mobile build stubbed.
func startJustRun(t *testing.T, env ...string) *justRun {
	t.Helper()
	s := newRecipeSandbox(t)
	sleeper := filepath.Join(t.TempDir(), "sleeper")
	src := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(src, []byte(sleeperSource), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("go", "build", "-o", sleeper, src).CombinedOutput(); err != nil {
		t.Fatalf("build sleeper: %v\n%s", err, out)
	}
	fakebin := filepath.Join(t.TempDir(), "bin")
	writeExecutable(t, filepath.Join(fakebin, "go"), sleeperGo)
	writeExecutable(t, filepath.Join(s.root, "scripts", "ios-sim"), iosSimStub)
	if err := os.MkdirAll(filepath.Join(s.root, "apps", "mobile"), 0o750); err != nil {
		t.Fatal(err)
	}
	// just runs the recipe in the resolved root (/private/var on macOS, not /var).
	root, err := filepath.EvalSymlinks(s.root)
	if err != nil {
		t.Fatal(err)
	}
	r := &justRun{
		done:    make(chan error, 1),
		backend: "^" + root + "/bin/(api|worker)$",
		started: filepath.Join(t.TempDir(), "mobile.started"),
		output:  &strings.Builder{},
	}
	r.cmd = exec.Command("just", "run")
	r.cmd.Dir = s.root
	r.cmd.Env = append(append(s.env, "SLEEPER="+sleeper, "MOBILE_STARTED="+r.started), env...)
	for i, kv := range r.cmd.Env {
		if path, ok := strings.CutPrefix(kv, "PATH="); ok {
			r.cmd.Env[i] = "PATH=" + fakebin + string(os.PathListSeparator) + path
		}
	}
	r.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	r.cmd.Stdout, r.cmd.Stderr = r.output, r.output
	if err := r.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { r.done <- r.cmd.Wait() }()
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("just run output:\n%s", r.output.String())
		}
		_ = syscall.Kill(-r.cmd.Process.Pid, syscall.SIGKILL)
		_ = exec.Command("pkill", "-KILL", "-f", r.backend).Run()
	})
	waitFor(t, "api, worker and the mobile build to start", func() bool {
		_, err := os.Stat(r.started)
		return err == nil
	})
	return r
}

func (r *justRun) waitExit(t *testing.T) {
	t.Helper()
	select {
	case <-r.done:
	case <-time.After(20 * time.Second):
		t.Fatal("just run did not exit in 20s")
	}
	waitFor(t, "api and worker to exit", func() bool {
		return exec.Command("pgrep", "-f", r.backend).Run() != nil
	})
}

func TestJustRun_ctrlCStopsTheBackend(t *testing.T) {
	r := startJustRun(t)

	// A terminal's Ctrl-C: SIGINT to the whole foreground process group.
	if err := syscall.Kill(-r.cmd.Process.Pid, syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	r.waitExit(t)
}

// A failed iOS build ends `just run`; the backend it started in the background must not
// outlive it.
func TestJustRun_failedMobileBuildStopsTheBackend(t *testing.T) {
	r := startJustRun(t, "MOBILE_EXIT=65")

	r.waitExit(t)
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
