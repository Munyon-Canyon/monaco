package ci_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestProbeListResolved(t *testing.T) {
	script := "probe-list-resolved.sh"
	dir := t.TempDir()
	derived := filepath.Join(dir, "derived")
	if err := os.MkdirAll(filepath.Join(derived, "SourcePackages"), 0o755); err != nil {
		t.Fatal(err)
	}
	logWith := filepath.Join(dir, "resolved.log")
	if err := os.WriteFile(logWith, []byte("Resolve Package Graph\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	logWithout := filepath.Join(dir, "list.log")
	if err := os.WriteFile(logWithout, []byte("Information about project \"Monaco\":\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	emptyDerived := filepath.Join(dir, "empty")
	if err := os.MkdirAll(emptyDerived, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		log     string
		derived string
		want    string
	}{
		{name: "resolve line", log: logWith, derived: emptyDerived, want: "yes"},
		{name: "source packages directory", log: logWithout, derived: derived, want: "yes"},
		{name: "list only", log: logWithout, derived: emptyDerived, want: "no"},
		{name: "missing log", log: filepath.Join(dir, "absent.log"), derived: emptyDerived, want: "no"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("bash", script, tc.log, tc.derived)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("classify: %v\n%s", err, out)
			}
			if got := strings.TrimSpace(string(out)); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestToolchainProbeRejectsOrder(t *testing.T) {
	cmd := exec.Command("bash", "toolchain-probe.sh")
	cmd.Env = append(os.Environ(), "PROBE_ORDER=sideways")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected a bad probe-order to fail\n%s", out)
	}
	if !strings.Contains(string(out), "probe-order must be pinned-first or default-first") {
		t.Fatalf("missing order error\n%s", out)
	}
}

func TestStampLogPrefixesTime(t *testing.T) {
	cmd := exec.Command("bash", "stamp-log.sh")
	cmd.Stdin = strings.NewReader("hello\nworld\n")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines: %#v", lines)
	}
	stamp := regexp.MustCompile(`^[0-9]{2}:[0-9]{2}:[0-9]{2} `)
	for _, line := range lines {
		if !stamp.MatchString(line) {
			t.Fatalf("line %q has no time prefix", line)
		}
	}
	if !strings.HasSuffix(lines[0], " hello") || !strings.HasSuffix(lines[1], " world") {
		t.Fatalf("body changed: %#v", lines)
	}
}
