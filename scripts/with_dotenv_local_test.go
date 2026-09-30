package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const argsRecordingDotenvx = `#!/usr/bin/env bash
printf '%s\n' "$@" > "$DOTENVX_ARGS"
while [[ "$1" != "--" ]]; do shift; done
shift
DATABASE_URL=postgres://fake exec "$@"
`

const fakeKeysFile = "DOTENV_PRIVATE_KEY_LOCAL=\"fake-key-for-tests\"\n"

type dotenvSandbox struct {
	primary, worktree, argsFile string
	env                         []string
}

func newDotenvSandbox(t *testing.T) dotenvSandbox {
	t.Helper()
	base := t.TempDir()
	primary := filepath.Join(base, "primary")
	worktree := filepath.Join(primary, ".worktrees", "wt")
	fakebin := filepath.Join(base, "bin")
	writeExecutable(t, filepath.Join(fakebin, "dotenvx"), argsRecordingDotenvx)
	copyFile(t, filepath.Join(repoRoot(t), "scripts", "with-dotenv-local.sh"), filepath.Join(primary, "scripts", "with-dotenv-local.sh"))
	writeFile(t, filepath.Join(primary, ".gitignore"), ".env.keys\n.env.local\n.worktrees/\n")
	s := dotenvSandbox{primary: primary, worktree: worktree, argsFile: filepath.Join(base, "args"), env: append(os.Environ(),
		"PATH="+fakebin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"DOTENVX_ARGS="+filepath.Join(base, "args"),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
	)}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "."},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "init"},
		{"worktree", "add", "-q", worktree},
	} {
		s.git(t, args...)
	}
	writeFile(t, filepath.Join(worktree, ".env.local"), "DOTENV_PUBLIC_KEY_LOCAL=\"fake\"\n")
	return s
}

func (s dotenvSandbox) git(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = s.primary
	cmd.Env = s.env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func (s dotenvSandbox) run(t *testing.T, extraEnv ...string) (stdout, stderr string, dotenvxArgs []string) {
	t.Helper()
	cmd := exec.Command(filepath.Join(s.worktree, "scripts", "with-dotenv-local.sh"), "env")
	cmd.Env = append(append([]string{}, s.env...), extraEnv...)
	var out, errOut strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		t.Fatalf("with-dotenv-local.sh: %v\n%s", err, errOut.String())
	}
	raw, err := os.ReadFile(s.argsFile)
	if err != nil {
		t.Fatal(err)
	}
	return out.String(), errOut.String(), strings.Split(strings.TrimSpace(string(raw)), "\n")
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func keysFileArg(args []string) (string, bool) {
	for i, a := range args {
		if a == "--" {
			break
		}
		if a == "-fk" && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

func TestWithDotenvLocal_resolvesKeySource(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		primaryKeys  string
		localKeys    string
		env          []string
		wantKeysFile func(s dotenvSandbox) string
		wantSource   string
	}{
		{
			name:         "worktree uses the primary clone keys file",
			primaryKeys:  fakeKeysFile,
			wantKeysFile: func(s dotenvSandbox) string { return filepath.Join(s.primary, ".env.keys") },
			wantSource:   "primary clone",
		},
		{
			name:         "local keys file wins over the primary clone",
			primaryKeys:  fakeKeysFile,
			localKeys:    fakeKeysFile,
			wantKeysFile: func(s dotenvSandbox) string { return filepath.Join(s.worktree, ".env.keys") },
			wantSource:   "in this checkout",
		},
		{
			name:       "no keys file leaves env and Armor to dotenvx",
			wantSource: "env or Dotenvx Armor",
		},
		{
			name:        "keys file without the local key is skipped",
			primaryKeys: "DOTENV_PRIVATE_KEY_PRODUCTION=\"fake-key-for-tests\"\n",
			wantSource:  "env or Dotenvx Armor",
		},
		{
			name:         "keys file and env var both present pass the file and keep the env",
			primaryKeys:  fakeKeysFile,
			env:          []string{"DOTENV_PRIVATE_KEY_LOCAL=fake-env-key"},
			wantKeysFile: func(s dotenvSandbox) string { return filepath.Join(s.primary, ".env.keys") },
			wantSource:   "primary clone",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newDotenvSandbox(t)
			if tc.primaryKeys != "" {
				writeFile(t, filepath.Join(s.primary, ".env.keys"), tc.primaryKeys)
			}
			if tc.localKeys != "" {
				writeFile(t, filepath.Join(s.worktree, ".env.keys"), tc.localKeys)
			}
			stdout, stderr, args := s.run(t, tc.env...)

			if strings.Count(stdout, "DATABASE_URL=") != 1 {
				t.Errorf("env output has no single DATABASE_URL:\n%s", stdout)
			}
			if got := strings.Count(stderr, "key source:"); got != 1 || !strings.Contains(stderr, tc.wantSource) {
				t.Errorf("stderr = %q, want one key source line naming %q", stderr, tc.wantSource)
			}
			if strings.Contains(stdout+stderr, "fake-key-for-tests") {
				t.Errorf("output leaks the keys file content")
			}
			for _, kv := range tc.env {
				if !strings.Contains(stdout, kv+"\n") {
					t.Errorf("env var %s did not reach dotenvx", kv)
				}
			}
			got, ok := keysFileArg(args)
			if tc.wantKeysFile == nil {
				if ok {
					t.Errorf("dotenvx args = %q, want no -fk", args)
				}
				return
			}
			want := tc.wantKeysFile(s)
			if !ok || !sameFile(t, got, want) {
				t.Errorf("dotenvx -fk = %q, want %q (args %q)", got, want, args)
			}
		})
	}
}

const getRecordingDotenvx = `#!/usr/bin/env bash
{
  printf '%s\n' "$@"
  printf '\n'
} >> "$DOTENVX_ARGS"
case "$2" in
  PRIVY_APP_ID) printf '%s' 'app-fake' ;;
  PRIVY_APP_CLIENT_ID) printf '%s' 'client-fake' ;;
  MONACO_STAGING_API_BASE_URL|MONACO_PRODUCTION_API_BASE_URL) printf '%s' 'https://example.test' ;;
esac
`

func installEnsureFixture(t *testing.T, s dotenvSandbox) {
	t.Helper()
	copyFile(t, filepath.Join(repoRoot(t), "scripts", "ensure-ios-privy-config.sh"), filepath.Join(s.worktree, "scripts", "ensure-ios-privy-config.sh"))
	var dotenvx string
	for _, kv := range s.env {
		if path, ok := strings.CutPrefix(kv, "PATH="); ok {
			dotenvx = filepath.Join(strings.Split(path, string(os.PathListSeparator))[0], "dotenvx")
		}
	}
	if dotenvx == "" {
		t.Fatal("sandbox PATH missing")
	}
	writeExecutable(t, dotenvx, getRecordingDotenvx)
}

func dotenvxInvocations(t *testing.T, s dotenvSandbox) [][]string {
	t.Helper()
	raw, err := os.ReadFile(s.argsFile)
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for _, block := range strings.Split(string(raw), "\n\n") {
		args := strings.Split(strings.TrimSuffix(block, "\n"), "\n")
		if len(args) == 0 || args[0] == "" {
			continue
		}
		out = append(out, args)
	}
	if len(out) == 0 {
		t.Fatal("dotenvx was not invoked")
	}
	return out
}

func TestEnsureIosPrivyConfig_worktreeUsesPrimaryCloneKeys(t *testing.T) {
	t.Parallel()
	s := newDotenvSandbox(t)
	writeFile(t, filepath.Join(s.primary, ".env.keys"), fakeKeysFile)
	installEnsureFixture(t, s)

	stdout, stderr := runEnsure(t, s, filepath.Join(s.worktree, "scripts", "ensure-ios-privy-config.sh"), "generate")
	if strings.Contains(stdout+stderr, "fake-key-for-tests") {
		t.Fatal("output leaks the keys file content")
	}
	if !strings.Contains(stderr, "primary clone") {
		t.Fatalf("stderr = %q, want the primary clone key source", stderr)
	}
	wantKeys := filepath.Join(s.primary, ".env.keys")
	for _, args := range dotenvxInvocations(t, s) {
		got, ok := keysFileArg(args)
		if !ok || !sameFile(t, got, wantKeys) {
			t.Errorf("dotenvx -fk = %q, want %q (args %q)", got, wantKeys, args)
		}
	}

	generated := filepath.Join(s.worktree, "apps", "mobile", "Config", "Privy.local.xcconfig")
	body, err := os.ReadFile(generated)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(body), "client-") != 1 {
		t.Fatalf("generated config client- count = %d, want 1", strings.Count(string(body), "client-"))
	}
}

func TestEnsureIosPrivyConfig_noKeysFileOmitsFk(t *testing.T) {
	t.Parallel()
	s := newDotenvSandbox(t)
	installEnsureFixture(t, s)

	_, stderr := runEnsure(t, s, filepath.Join(s.worktree, "scripts", "ensure-ios-privy-config.sh"), "generate")
	if strings.Count(stderr, "key source:") != 1 || !strings.Contains(stderr, "env or Dotenvx Armor") {
		t.Fatalf("stderr = %q, want the env or Armor key source", stderr)
	}
	for _, args := range dotenvxInvocations(t, s) {
		if _, ok := keysFileArg(args); ok {
			t.Errorf("dotenvx args = %q, want no -fk", args)
		}
	}
}

func TestEnsureIosPrivyConfig_secondSourceKeepsFk(t *testing.T) {
	t.Parallel()
	s := newDotenvSandbox(t)
	writeFile(t, filepath.Join(s.primary, ".env.keys"), fakeKeysFile)
	installEnsureFixture(t, s)
	script := filepath.Join(s.worktree, "scripts", "ensure-ios-privy-config.sh")

	_, stderr := runEnsure(t, s, "bash", "-c", "source \"$1\"; generate_xcconfig; source \"$1\"; generate_xcconfig", "bash", script)
	if strings.Count(stderr, "primary clone") != 2 {
		t.Fatalf("stderr = %q, want two primary-clone key source lines", stderr)
	}
	wantKeys := filepath.Join(s.primary, ".env.keys")
	for _, args := range dotenvxInvocations(t, s) {
		got, ok := keysFileArg(args)
		if !ok || !sameFile(t, got, wantKeys) {
			t.Errorf("dotenvx -fk = %q, want %q (args %q)", got, wantKeys, args)
		}
	}
}

func runEnsure(t *testing.T, s dotenvSandbox, name string, args ...string) (stdout, stderr string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Env = s.env
	var out, errOut strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s: %v\n%s", name, err, errOut.String())
	}
	return out.String(), errOut.String()
}

func sameFile(t *testing.T, a, b string) bool {
	t.Helper()
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	return os.SameFile(ai, bi)
}
