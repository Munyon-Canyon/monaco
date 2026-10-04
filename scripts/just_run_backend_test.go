package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
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

type recipeSandbox struct {
	root, calls string
	env         []string
}

func newRecipeSandbox(t *testing.T) recipeSandbox {
	t.Helper()
	if _, err := exec.LookPath("just"); err != nil {
		t.Skip("just is not on PATH")
	}
	repo := repoRoot(t)
	root := t.TempDir()
	copyFile(t, filepath.Join(repo, "Justfile"), filepath.Join(root, "Justfile"))
	for _, script := range []string{"with-dotenv-local.sh", "require-docker.sh", "run-with-logs.sh"} {
		copyFile(t, filepath.Join(repo, "scripts", script), filepath.Join(root, "scripts", script))
	}
	writeExecutable(t, filepath.Join(root, ".env.local"), "MONACO_ENV=local\n")
	if err := os.MkdirAll(filepath.Join(root, "apps", "backend"), 0o750); err != nil {
		t.Fatal(err)
	}
	fakebin := filepath.Join(t.TempDir(), "bin")
	writeExecutable(t, filepath.Join(fakebin, "go"), fakeGo)
	writeExecutable(t, filepath.Join(fakebin, "docker"), recordingStub)
	writeExecutable(t, filepath.Join(fakebin, "dotenvx"), fakeDotenvx)
	writeExecutable(t, filepath.Join(root, ".bin", "atlas"), recordingStub)
	calls := filepath.Join(t.TempDir(), "calls")
	return recipeSandbox{root: root, calls: calls, env: append(os.Environ(),
		"PATH="+fakebin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CALLS="+calls,
		"RECORDING_STUB="+recordingStub,
		"MONACO_LOG_DIR=",
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

func TestJustMigrateDb_appliesThenReportsTheRevision(t *testing.T) {
	s := newRecipeSandbox(t)

	calls := s.just(t, "migrate", "db")

	want := []string{"monacoctl migrate apply", "monacoctl migrate status"}
	got := calls[len(calls)-len(want):]
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls end with %q, want %q\nall calls:\n%s", got, want, strings.Join(calls, "\n"))
	}
}
