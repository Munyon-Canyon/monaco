package agents

import (
	"strings"
	"testing"
)

func TestDescribe_cutsAt141Runes(t *testing.T) {
	t.Parallel()
	prefix := []rune("root-check by sonnet: ")
	body := strings.Repeat("å", 141-len(prefix))
	got := describe(RootCheck, "sonnet", body)
	if len([]rune(got)) != 140 {
		t.Fatalf("len=%d %q", len([]rune(got)), got)
	}
}

func TestCarried_cutsAt141Runes(t *testing.T) {
	t.Parallel()
	prefix := []rune("carried from aaaaaaa: ")
	body := strings.Repeat("b", 141-len(prefix))
	got := carried(strings.Repeat("a", 40), body)
	if len([]rune(got)) != 140 {
		t.Fatalf("len=%d %q", len([]rune(got)), got)
	}
}

func TestLoadVerdict_reportsMissingAndCorruptFiles(t *testing.T) {
	t.Parallel()
	env := newFixture(t).Env(t)
	if _, err := env.loadVerdict(9); err == nil || !strings.Contains(err.Error(), "read verdict") {
		t.Fatalf("missing: %v", err)
	}
	writeFile(t, env.verdictPath(9), "{")
	if _, err := env.loadVerdict(9); err == nil || !strings.Contains(err.Error(), "decode verdict") {
		t.Fatalf("decode: %v", err)
	}
	writeFile(t, env.verdictPath(9), "{\"sha\":\"abc\"}\n")
	got, err := env.loadVerdict(9)
	if err != nil || got.SHA != "abc" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
