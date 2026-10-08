package ci_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFlakeTests_rerunsChangedTestFuncsWithTheFaultpointsTag(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"apps/backend/internal/db/lock_test.go":                                     "package db\n\nfunc TestLock(t *testing.T) {}\nfunc TestUnlock(t *testing.T) {}\nfunc helper() {}\n",
		"apps/backend/internal/db/note.go":                                          "package db\n",
		"apps/backend/internal/app/helpers_test.go":                                 "package app\n",
		"apps/backend/internal/platform/bus/chaos/testdata/counter/counter_test.go": "package counter\n\nfunc TestCounterWithoutDedupe(t *testing.T) {}\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	script := filepath.Join(repoRoot(t), "scripts", "ci", "flake-tests.sh")
	for _, tc := range []struct {
		files []string
		want  string
	}{
		{
			[]string{"apps/backend/internal/db/lock_test.go", "apps/backend/internal/db/note.go"},
			"go test -tags faultpoints -short -count=20 -cpu=1,2 -run '^(TestLock|TestUnlock)$' ./internal/db",
		},
		{
			[]string{"apps/backend/internal/app/helpers_test.go"},
			"go test -tags faultpoints -short -count=20 -cpu=1,2 ./internal/app",
		},
		{
			[]string{"apps/backend/internal/platform/bus/chaos/testdata/counter/counter_test.go"},
			"",
		},
		{
			[]string{"apps/backend/internal/platform/bus/chaos/testdata/counter/counter_test.go", "apps/backend/internal/app/helpers_test.go"},
			"go test -tags faultpoints -short -count=20 -cpu=1,2 ./internal/app",
		},
	} {
		cmd := exec.Command("bash", append([]string{script, "--print", "--root", root}, tc.files...)...)
		cmd.Env = append(os.Environ(), "PYENV_VERSION=system")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("flake-tests.sh %v: %v\n%s", tc.files, err, out)
		}
		if got := strings.TrimSpace(string(out)); got != tc.want {
			t.Errorf("flake-tests.sh %v printed\n%s\nwant\n%s", tc.files, got, tc.want)
		}
	}
}

func TestFlakeTests_listsPackagesAndShardsByPackage(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"apps/backend/internal/db/lock_test.go":                                     "package db\n\nfunc TestLock(t *testing.T) {}\n",
		"apps/backend/internal/db/unlock_test.go":                                   "package db\n\nfunc TestUnlock(t *testing.T) {}\n",
		"apps/backend/internal/app/helpers_test.go":                                 "package app\n",
		"apps/backend/internal/platform/bus/chaos/testdata/counter/counter_test.go": "package counter\n\nfunc TestCounterWithoutDedupe(t *testing.T) {}\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	script := filepath.Join(repoRoot(t), "scripts", "ci", "flake-tests.sh")
	files := []string{
		"apps/backend/internal/db/lock_test.go",
		"apps/backend/internal/app/helpers_test.go",
		"apps/backend/internal/db/unlock_test.go",
		"apps/backend/internal/platform/bus/chaos/testdata/counter/counter_test.go",
		"apps/backend/internal/db/note.go",
	}
	for _, tc := range []struct {
		name    string
		args    []string
		want    string
		wantErr string
	}{
		{
			"list-packages prints each package once in first-seen order, without testdata or non-test files",
			[]string{"--list-packages"},
			"./internal/db\n./internal/app",
			"",
		},
		{
			"package prints only that package's command",
			[]string{"--print", "--package", "./internal/db"},
			"go test -tags faultpoints -short -count=20 -cpu=1,2 -run '^(TestLock|TestUnlock)$' ./internal/db",
			"",
		},
		{
			"package without Test funcs runs whole",
			[]string{"--print", "--package", "./internal/app"},
			"go test -tags faultpoints -short -count=20 -cpu=1,2 ./internal/app",
			"",
		},
		{
			"unknown package fails naming the package",
			[]string{"--print", "--package", "./internal/nope"},
			"",
			"flake-tests.sh: no changed tests in ./internal/nope",
		},
		{
			"testdata package is not a shard",
			[]string{"--print", "--package", "./internal/platform/bus/chaos/testdata/counter"},
			"",
			"flake-tests.sh: no changed tests in ./internal/platform/bus/chaos/testdata/counter",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{script, "--root", root}, tc.args...)
			cmd := exec.Command("bash", append(args, files...)...)
			cmd.Env = append(os.Environ(), "PYENV_VERSION=system")
			out, err := cmd.CombinedOutput()
			if tc.wantErr != "" {
				if exitErr, ok := err.(*exec.ExitError); !ok || exitErr.ExitCode() != 1 {
					t.Fatalf("flake-tests.sh %v: err %v, want exit 1\n%s", tc.args, err, out)
				}
				if got := strings.TrimSpace(string(out)); got != tc.wantErr {
					t.Errorf("flake-tests.sh %v printed\n%s\nwant\n%s", tc.args, got, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("flake-tests.sh %v: %v\n%s", tc.args, err, out)
			}
			if got := strings.TrimSpace(string(out)); got != tc.want {
				t.Errorf("flake-tests.sh %v printed\n%s\nwant\n%s", tc.args, got, tc.want)
			}
		})
	}
}
