package ci_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const (
	labelAction = "(github.event.action == 'labeled' || github.event.action == 'unlabeled')"
	queueLabel  = "!(" + labelAction + " && (github.event.label.name == 'merge-queue' || " +
		"github.event.label.name == 'fast-track'))"
	mergeQueue = "!(" + labelAction + " && github.event.label.name == 'merge-queue')"
)

func jobBlocks(t *testing.T, path string) (string, map[string]string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	head, jobs, _ := strings.Cut(string(raw), "\njobs:\n")
	blocks := map[string]string{}
	name := ""
	for _, line := range strings.Split(jobs, "\n") {
		if m := jobHeader.FindStringSubmatch(line); m != nil {
			name = m[1]
		}
		blocks[name] += strings.TrimSpace(line) + " "
	}
	return head, blocks
}

func TestPRFormat_queueLabelEventsSkipTheRequiredChecks(t *testing.T) {
	head, jobs := jobBlocks(t, filepath.Join(repoRoot(t), ".github", "workflows", "pr-format.yml"))
	for job, want := range map[string]string{
		"pr-format":    queueLabel,
		"gate-changes": queueLabel,
		"changelog":    queueLabel,
		"pr-size":      mergeQueue,
	} {
		if !strings.Contains(jobs[job], want) {
			t.Errorf("%s does not skip with %s:\n%s", job, want, jobs[job])
		}
	}
	if strings.Contains(jobs["pr-size"], "github.event.label.name == 'fast-track'") {
		t.Error("pr-size skips fast-track, which it enforces")
	}
	group := regexp.MustCompile(`(?m)^  group: (.+)$`).FindStringSubmatch(head)
	if group == nil || !strings.Contains(group[1], labelAction+" && (github.event.label.name == 'merge-queue' || "+
		"github.event.label.name == 'fast-track') && '-label' || ''") {
		t.Errorf("queue-label runs share the concurrency group of real runs: %v", group)
	}
}
