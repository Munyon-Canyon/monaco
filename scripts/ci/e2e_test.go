package ci_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runE2E(t *testing.T, leg string) (int, string) {
	t.Helper()
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	stub := filepath.Join(dir, "monacoctl")
	writeExecutable(t, stub, "#!/usr/bin/env bash\n"+
		"if [ \"$1 $2\" = 'flows crash-points' ]; then echo p1; echo p2; echo p3; exit 0; fi\n"+
		"printf '%s\\n' \"$*\" >> "+calls+"\n")
	cmd := exec.Command("bash", "e2e.sh")
	cmd.Env = append(os.Environ(), "E2E_MONACOCTL="+stub, "E2E_LEG="+leg)
	code := 0
	if err := cmd.Run(); err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatal(err)
		}
		code = exitErr.ExitCode()
	}
	log, err := os.ReadFile(calls)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return code, string(log)
}

func TestE2E_runsTheUnitsOfOneLeg(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		leg  string
		want []string
	}{
		"leg 1 of 3": {"1/3", []string{"verify all", "verify all --crash-at p3"}},
		"leg 2 of 3": {"2/3", []string{"verify all --crash-at p1"}},
		"leg 3 of 3": {"3/3", []string{"verify all --crash-at p2"}},
		"one leg":    {"1/1", []string{"verify all", "verify all --crash-at p1", "verify all --crash-at p2", "verify all --crash-at p3"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			code, log := runE2E(t, c.leg)
			if code != 0 {
				t.Fatalf("exit = %d, want 0", code)
			}
			if want := strings.Join(c.want, "\n") + "\n"; log != want {
				t.Fatalf("verify calls = %q, want %q", log, want)
			}
		})
	}
}

func TestE2E_refusesALegOutsideTheRange(t *testing.T) {
	t.Parallel()
	for _, leg := range []string{"0/3", "4/3", "x", "1/"} {
		code, log := runE2E(t, leg)
		if code != 2 || log != "" {
			t.Errorf("leg %q: exit = %d, calls = %q, want exit 2 and no calls", leg, code, log)
		}
	}
}
