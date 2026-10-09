package scripts_test

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"
)

const recordingStub = "#!/usr/bin/env bash\nprintf '%s\\n' \"$(basename \"$0\") $*\" >> \"$CALLS\"\n"

const fakeGo = `#!/usr/bin/env bash
printf 'go %s\n' "$*" >> "$CALLS"
out="$PWD/../../bin"
mkdir -p "$out"
for name in api worker monacoctl; do
  printf '%s' "$RECORDING_STUB" > "$out/$name"
  chmod +x "$out/$name"
done
`

const fakeDotenvx = `#!/usr/bin/env bash
while [[ "$1" != "--" ]]; do shift; done
shift
exec "$@"
`

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

// A real binary, not a script: it listens on the --port it is given, as vite does, until SIGTERM.
const listenerSource = `package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	port := flag.Int("port", 0, "")
	flag.Bool("strictPort", false, "")
	flag.Parse()
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		os.Exit(1)
	}
	defer ln.Close()
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, os.Interrupt)
	<-c
}
`

// fakeNpx stands in for `npx vite`: it records where and how it was started, then becomes the listener.
const fakeNpx = `#!/usr/bin/env bash
printf 'npx %s | dir=%s api=%s privy=%s env=%s\n' "$*" "$(basename "$PWD")" "$VITE_MONACO_API_URL" "$VITE_PRIVY_APP_ID" "$VITE_PRIVY_ENV" >> "$CALLS"
shift
mkdir -p "$PWD/node_modules/.bin"
cp "$LISTENER" "$PWD/node_modules/.bin/vite"
exec "$PWD/node_modules/.bin/vite" "$@"
`

type recipeSandbox struct {
	root, calls string
	fundPort    int
	env         []string
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func listening(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// withoutNode returns PATH with npx, npm and node hidden: each directory that holds one is replaced
// by a directory of symlinks to everything else in it.
func withoutNode(t *testing.T, path string) string {
	t.Helper()
	var dirs []string
	for _, dir := range filepath.SplitList(path) {
		if _, err := os.Stat(filepath.Join(dir, "npx")); err != nil {
			dirs = append(dirs, dir)
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		shadow := t.TempDir()
		for _, e := range entries {
			if n := e.Name(); n != "npx" && n != "npm" && n != "node" {
				_ = os.Symlink(filepath.Join(dir, n), filepath.Join(shadow, n))
			}
		}
		dirs = append(dirs, shadow)
	}
	return strings.Join(dirs, string(os.PathListSeparator))
}

func newRecipeSandbox(t *testing.T) recipeSandbox {
	t.Helper()
	if _, err := exec.LookPath("just"); err != nil {
		t.Skip("just is not on PATH")
	}
	repo := repoRoot(t)
	root := t.TempDir()
	copyFile(t, filepath.Join(repo, "Justfile"), filepath.Join(root, "Justfile"))
	for _, script := range []string{"with-dotenv-local.sh", "require-docker.sh", "run-with-logs.sh", "kill-listeners.sh"} {
		copyFile(t, filepath.Join(repo, "scripts", script), filepath.Join(root, "scripts", script))
	}
	writeExecutable(t, filepath.Join(root, ".env.local"), "MONACO_ENV=local\n")
	if err := os.MkdirAll(filepath.Join(root, "apps", "backend"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "apps", "web", "node_modules"), 0o750); err != nil {
		t.Fatal(err)
	}
	listener := filepath.Join(t.TempDir(), "listener")
	src := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(src, []byte(listenerSource), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("go", "build", "-o", listener, src).CombinedOutput(); err != nil {
		t.Fatalf("build listener: %v\n%s", err, out)
	}
	fundPort := freePort(t)
	fakebin := filepath.Join(t.TempDir(), "bin")
	writeExecutable(t, filepath.Join(fakebin, "go"), fakeGo)
	writeExecutable(t, filepath.Join(fakebin, "npx"), fakeNpx)
	writeExecutable(t, filepath.Join(fakebin, "npm"), recordingStub+"exit ${NPM_EXIT:-0}\n")
	writeExecutable(t, filepath.Join(fakebin, "docker"), recordingStub)
	writeExecutable(t, filepath.Join(fakebin, "dotenvx"), fakeDotenvx)
	writeExecutable(t, filepath.Join(root, ".bin", "atlas"), recordingStub)
	calls := filepath.Join(t.TempDir(), "calls")
	t.Cleanup(func() {
		_ = exec.Command(filepath.Join(root, "scripts", "kill-listeners.sh"), fmt.Sprint(fundPort)).Run()
	})
	return recipeSandbox{root: root, calls: calls, fundPort: fundPort, env: append(os.Environ(),
		"PATH="+fakebin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CALLS="+calls,
		// Long enough for the fund page, started beside api and worker, to be recorded before they exit.
		"RECORDING_STUB="+recordingStub+"[[ -z \"${MONACO_FUND_PAGE_PORT:-}\" || \"$(basename \"$0\")\" == monacoctl ]] || echo \"leak $(basename \"$0\")\" >> \"$CALLS\"\nsleep 1\n",
		"MONACO_LOG_DIR=",
		"LISTENER="+listener,
		fmt.Sprintf("MONACO_FUND_PAGE_PORT=%d", fundPort),
		"PRIVY_APP_ID=privy-test",
	)}
}

func (s recipeSandbox) just(t *testing.T, args ...string) []string {
	t.Helper()
	cmd := exec.Command("just", args...)
	cmd.Dir = s.root
	cmd.Env = s.env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("just %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	data, err := os.ReadFile(s.calls)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func TestJustRunBackend_startsTheServicesWithoutMigrating(t *testing.T) {
	s := newRecipeSandbox(t)

	calls := s.just(t, "run", "backend")

	var started []string
	for _, call := range calls {
		if strings.Contains(call, "migrate") || strings.HasPrefix(call, "atlas") {
			t.Fatalf("just run backend ran %q, want no migration\nall calls:\n%s", call, strings.Join(calls, "\n"))
		}
		if name := strings.TrimSpace(call); name == "api" || name == "worker" {
			started = append(started, name)
		}
	}
	if len(started) != 2 {
		t.Fatalf("started %v, want api and worker\nall calls:\n%s", started, strings.Join(calls, "\n"))
	}
}

func TestJustMigrateDb_appliesThenReportsTheRevisionThenAppliesTheStreams(t *testing.T) {
	s := newRecipeSandbox(t)

	calls := s.just(t, "migrate", "db")

	want := []string{"monacoctl migrate apply", "monacoctl migrate status", "monacoctl bus apply"}
	got := calls[len(calls)-len(want):]
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls end with %q, want %q\nall calls:\n%s", got, want, strings.Join(calls, "\n"))
	}
}

func fundPageCalls(calls []string) []string {
	var got []string
	for _, call := range calls {
		if strings.HasPrefix(call, "npx ") {
			got = append(got, call)
		}
	}
	return got
}

func (s recipeSandbox) justWithEnv(t *testing.T, env []string, args ...string) (string, []string) {
	t.Helper()
	cmd := exec.Command("just", args...)
	cmd.Dir = s.root
	cmd.Env = append(s.env, env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("just %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	data, _ := os.ReadFile(s.calls)
	return string(out), strings.Split(strings.TrimSpace(string(data)), "\n")
}

func TestJustRunBackend_startsTheFundPageAgainstTheLocalApi(t *testing.T) {
	s := newRecipeSandbox(t)

	calls := s.just(t, "run", "backend")

	for _, call := range calls {
		if strings.HasPrefix(call, "leak ") {
			t.Fatalf("%s was started with MONACO_FUND_PAGE_PORT, which the backend rejects as an unknown variable", call)
		}
	}
	waitFor(t, "the fund page to stop when the recipe exits", func() bool { return !listening(s.fundPort) })
	want := fmt.Sprintf("npx vite --port %d --strictPort | dir=web api=http://localhost:8080 privy=privy-test env=sandbox", s.fundPort)
	if got := fundPageCalls(calls); len(got) != 1 || got[0] != want {
		t.Fatalf("fund page calls = %q, want [%q]\nall calls:\n%s", got, want, strings.Join(calls, "\n"))
	}
}

func TestJustRunBackend_installsTheFundPageDependenciesWhenMissing(t *testing.T) {
	s := newRecipeSandbox(t)
	if err := os.Remove(filepath.Join(s.root, "apps", "web", "node_modules")); err != nil {
		t.Fatal(err)
	}

	calls := s.just(t, "run", "backend")

	install, vite := -1, -1
	for i, call := range calls {
		switch {
		case call == "npm ci":
			install = i
		case strings.HasPrefix(call, "npx "):
			vite = i
		}
	}
	if install < 0 || vite < install {
		t.Fatalf("want npm ci before vite\nall calls:\n%s", strings.Join(calls, "\n"))
	}
}

func TestJustRunBackend_skipsTheFundPageWhenThePortIsTaken(t *testing.T) {
	s := newRecipeSandbox(t)
	taken, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.fundPort))
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()

	calls := s.just(t, "run", "backend")

	if got := fundPageCalls(calls); len(got) != 0 {
		t.Fatalf("started the fund page on a taken port: %q", got)
	}
	var started int
	for _, call := range calls {
		if name := strings.TrimSpace(call); name == "api" || name == "worker" {
			started++
		}
	}
	if started != 2 {
		t.Fatalf("started %d of api and worker\nall calls:\n%s", started, strings.Join(calls, "\n"))
	}
	if !listening(s.fundPort) {
		t.Fatal("just run backend killed the listener it did not start")
	}
}

func TestJustRunBackend_startsWithoutNodeAndWarnsOnce(t *testing.T) {
	s := newRecipeSandbox(t)
	// The sandbox's fake npx sits first on PATH; take it out along with any real Node.
	i := slices.IndexFunc(s.env, func(kv string) bool { return strings.HasPrefix(kv, "PATH=") })
	for j := len(s.env) - 1; j >= 0; j-- {
		if strings.HasPrefix(s.env[j], "PATH=") {
			i = j
			break
		}
	}
	fake, rest, _ := strings.Cut(strings.TrimPrefix(s.env[i], "PATH="), string(os.PathListSeparator))
	if err := os.Remove(filepath.Join(fake, "npx")); err != nil {
		t.Fatal(err)
	}
	s.env[i] = "PATH=" + fake + string(os.PathListSeparator) + withoutNode(t, rest)

	out, calls := s.justWithEnv(t, nil, "run", "backend")

	const warning = "fund page not started: Node is not installed; card deposits will not open"
	if n := strings.Count(out, warning); n != 1 {
		t.Fatalf("warning printed %d times, want once\n%s", n, out)
	}
	var started int
	for _, call := range calls {
		if name := strings.TrimSpace(call); name == "api" || name == "worker" {
			started++
		}
	}
	if started != 2 {
		t.Fatalf("started %d of api and worker\nall calls:\n%s", started, strings.Join(calls, "\n"))
	}
}

// startFundListener listens on the sandbox's fund port, as this checkout's Vite when asVite and
// as an unrelated program otherwise.
func startFundListener(t *testing.T, s recipeSandbox, asVite bool) {
	t.Helper()
	listener := ""
	for _, kv := range s.env {
		if v, ok := strings.CutPrefix(kv, "LISTENER="); ok {
			listener = v
		}
	}
	if asVite {
		root, err := filepath.EvalSymlinks(s.root)
		if err != nil {
			t.Fatal(err)
		}
		vite := filepath.Join(root, "apps", "web", "node_modules", ".bin", "vite")
		writeExecutable(t, vite, "")
		data, err := os.ReadFile(listener)
		if err != nil {
			t.Fatal(err)
		}
		writeExecutable(t, vite, string(data))
		listener = vite
	}
	page := exec.Command(listener, "--port", fmt.Sprint(s.fundPort), "--strictPort")
	if err := page.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = page.Wait() }()
	t.Cleanup(func() { _ = page.Process.Kill() })
	waitFor(t, "the fund page to listen", func() bool { return listening(s.fundPort) })
}

func TestJustStopBackend_stopsThisCheckoutsFundPage(t *testing.T) {
	s := newRecipeSandbox(t)
	startFundListener(t, s, true)

	s.justWithEnv(t, nil, "stop", "backend")

	waitFor(t, "nothing to listen on the fund page port", func() bool { return !listening(s.fundPort) })
}

func TestJustStopBackend_leavesAnUnrelatedListenerOnTheFundPort(t *testing.T) {
	s := newRecipeSandbox(t)
	startFundListener(t, s, false)

	s.justWithEnv(t, nil, "stop", "backend")

	time.Sleep(500 * time.Millisecond)
	if !listening(s.fundPort) {
		t.Fatal("just stop backend killed a listener that is not this checkout's fund page")
	}
}

func TestJustRunBackend_skipsTheFundPageWhenOff(t *testing.T) {
	s := newRecipeSandbox(t)

	out, calls := s.justWithEnv(t, []string{"MONACO_FUND_PAGE_PORT=off"}, "run", "backend")

	if got := fundPageCalls(calls); len(got) != 0 {
		t.Fatalf("started the fund page while off: %q", got)
	}
	if strings.Contains(out, "fund page") {
		t.Fatalf("off should be silent, got:\n%s", out)
	}
	for _, call := range calls {
		if strings.HasPrefix(call, "leak ") {
			t.Fatalf("%s was started with MONACO_FUND_PAGE_PORT", call)
		}
	}
}

func TestJustRunBackend_keepsTheBackendWhenTheInstallFails(t *testing.T) {
	s := newRecipeSandbox(t)
	if err := os.Remove(filepath.Join(s.root, "apps", "web", "node_modules")); err != nil {
		t.Fatal(err)
	}

	out, calls := s.justWithEnv(t, []string{"NPM_EXIT=1"}, "run", "backend")

	if !strings.Contains(out, "fund page not started: npm ci failed; see ") {
		t.Fatalf("want the npm ci warning, got:\n%s", out)
	}
	if got := fundPageCalls(calls); len(got) != 0 {
		t.Fatalf("started the fund page after a failed install: %q", got)
	}
	var started int
	for _, call := range calls {
		if name := strings.TrimSpace(call); name == "api" || name == "worker" {
			started++
		}
	}
	if started != 2 {
		t.Fatalf("started %d of api and worker\nall calls:\n%s", started, strings.Join(calls, "\n"))
	}
}

func TestJustKillports_stopsTheFundPageAndOnlyOurPorts(t *testing.T) {
	s := newRecipeSandbox(t)
	// Never the default 8080 and 8081: a developer's own stack may be listening there.
	env := []string{
		fmt.Sprintf("MONACO_HTTP_ADDR=:%d", freePort(t)),
		fmt.Sprintf("MONACO_WORKER_HEALTH_ADDR=:%d", freePort(t)),
	}
	startFundListener(t, s, false)

	s.justWithEnv(t, env, "killports")

	waitFor(t, "nothing to listen on the fund page port", func() bool { return !listening(s.fundPort) })
}
