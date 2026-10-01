package ci_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildTimingSummary_sumsTaskTypes(t *testing.T) {
	root := filepath.Join("..", "..")
	script := filepath.Join(root, "scripts", "ci", "build-timing-summary.sh")
	log := filepath.Join(root, "scripts", "ci", "testdata", "build-timing-log.json")

	cmd := exec.Command("bash", script)
	cmd.Env = append(os.Environ(), "BUILD_TIMING_LOG="+log)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, want := range []string{
		"Build Timing Summary",
		"SwiftCompile (2 tasks) | 3.750 seconds",
		"SwiftDriver (1 task) | 3.000 seconds",
		"Ld (1 task) | 1.000 seconds",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("summary missing %q\n%s", want, text)
		}
	}
	if strings.Contains(text, "working") || strings.Contains(text, "swiftc") {
		t.Fatalf("non-task command leaked into the summary\n%s", text)
	}
	if strings.Contains(text, "2.400") || strings.Contains(text, "0.200") {
		t.Fatalf("child task was counted separately\n%s", text)
	}
	compile := strings.Index(text, "SwiftCompile")
	driver := strings.Index(text, "SwiftDriver")
	link := strings.Index(text, "Ld (")
	if compile > driver || driver > link {
		t.Fatalf("tasks are not ordered by duration\n%s", text)
	}
}
