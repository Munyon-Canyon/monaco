package marketing_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/cmd/monacoctl/marketing"
	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func TestMain_PrintsUsageForWhatItDoesNotKnow(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"bogus"}, {"watch", "--bogus"}, {"watch", "extra"}, {"watch", "--help"}} {
		b := newBench(t, watchConfig)

		code, _, stderr := b.run(args...)

		if code != 2 || !strings.Contains(stderr, "usage: monacoctl marketing watch [--once] [--dry-run]") {
			t.Errorf("args %q: exit %d, stderr %q; want 2 and the usage line", args, code, stderr)
		}
	}
}

func TestMain_NamesTheMissingKeysWithoutTheirValues(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)
	b.deps.Environ = []string{"X_BEARER_TOKEN=" + xToken, "TELEGRAM_CHAT_ID= \n"}

	code, _, stderr := b.run("watch", "--once")

	if code != 1 || !strings.Contains(stderr, "set TELEGRAM_BOT_TOKEN, TELEGRAM_CHAT_ID") ||
		strings.Contains(stderr, xToken) {
		t.Fatalf("exit %d, stderr %q; want 1 naming the two missing keys only", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(b.repo, ".build")); err == nil {
		t.Error("created .build before the keys were checked")
	}
}

func TestMain_TrimsKeysFromTheEnvironment(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)
	b.deps.Environ = []string{
		"X_BEARER_TOKEN=" + xToken + "\n",
		"TELEGRAM_BOT_TOKEN=" + botToken,
		"TELEGRAM_CHAT_ID=" + chatID,
	}

	b.run("watch", "--once")

	if got := b.x.sent()[0].auth; got != "Bearer "+xToken {
		t.Fatalf("Authorization = %q, want the token without its newline", got)
	}
}

func TestMain_ExplainsAMissingConfig(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)
	b.deps.Dir = "/nowhere-for-the-marketing-test"

	code, _, stderr := b.run("watch", "--dry-run")

	if code != 1 || !strings.Contains(stderr, "no .monaco/marketing.toml in /nowhere-for-the-marketing-test or above") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestMain_FindsTheConfigFromAnyFolderBelowIt(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)
	deep := filepath.Join(b.repo, "apps", "backend", "cmd")
	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{b.repo, deep} {
		b.deps.Dir = dir

		if code, _, stderr := b.run("watch", "--dry-run"); code != 0 {
			t.Errorf("from %s: exit %d, stderr %q", dir, code, stderr)
		}
	}
}

func TestMain_ExplainsABadConfig(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)
	b.rewriteConfig(strings.Replace(watchConfig, "daily_cap = 30", "daily_cap = 0", 1))

	code, _, stderr := b.run("watch", "--once")

	if code != 1 || !strings.Contains(stderr, ".monaco/marketing.toml: daily_cap must be positive") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestMain_RefusesACorruptStateFile(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)
	dir := filepath.Join(b.repo, ".build", "marketing")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := b.run("watch", "--once")

	if code != 1 || len(b.x.sent()) != 0 || len(b.tg.messages()) != 0 {
		t.Fatalf(
			"exit %d, %d X requests, %d messages, stderr %q; want 1 and no calls",
			code,
			len(b.x.sent()),
			len(b.tg.messages()),
			stderr,
		)
	}
}

func TestMain_ReportsABuildFolderItCannotUse(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)
	if err := os.WriteFile(filepath.Join(b.repo, ".build"), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := b.run("watch", "--once")

	if code != 1 || !strings.Contains(stderr, "storage_unavailable") {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
}

func TestDryRun_PrintsTheScoredFixtureWithoutTouchingAnything(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)
	b.deps.Environ = nil

	code, stdout, stderr := b.run("watch", "--dry-run")

	if code != 0 || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	header := "dry run: 14 posts in the fixture, scored as of 2026-10-18T10:05:00Z; 10 would alert (min score 5, cap 30 a day)\n"
	if !strings.HasPrefix(stdout, header) {
		t.Fatalf("output starts %q, want %q", stdout[:min(len(stdout), len(header))], header)
	}
	first := "\n  9.97  alert  @sofia_sol · 7800 followers · FOMO app\n" +
		"        Is the FOMO app just copy trading with extra steps?\n" +
		"        angle: Copy trading copies one person.\n" +
		"        https://x.com/sofia_sol/status/1847000000000014000\n"
	if !strings.Contains(stdout, first) {
		t.Errorf("output lacks the top block:\n%s", stdout)
	}
	for _, want := range []string{
		"  6.73  alert  @pablo_trades · 18500 followers · group chat investing\n",
		"  4.10  below min score  @bigfund · 210000 followers · copy trading\n",
		"  2.15  below min score  @etf_ellie · 420 followers · copy trading\n",
		"  skip  cashtags only  @traderjoe  https://x.com/traderjoe/status/1847000000000010000\n",
		"  skip  under the follower floor  @stocksbot24  https://x.com/stocksbot24/status/1847000000000012000\n",
		"@clubtreasurer · 3200 followers · group chat investing, investment club\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	if strings.Index(stdout, "@sofia_sol") > strings.Index(stdout, "@lucia_ahorra") ||
		strings.Index(stdout, "@pablo_trades") > strings.Index(stdout, "@bigfund") {
		t.Error("blocks are not best first, alerts before the rest")
	}
	if len(b.x.sent())+len(b.tg.messages()) != 0 {
		t.Error("the dry run called X or Telegram")
	}
	if _, err := os.Stat(filepath.Join(b.repo, ".build")); err == nil {
		t.Error("the dry run wrote .build")
	}
}

func TestDryRun_MarksPostsOverTheDailyCap(t *testing.T) {
	t.Parallel()
	b := newBench(t, strings.Replace(watchConfig, "daily_cap = 30", "daily_cap = 2", 1))

	_, stdout, _ := b.run("watch", "--dry-run")

	if !strings.Contains(stdout, "2 would alert (min score 5, cap 2 a day)") ||
		!strings.Contains(stdout, "  9.82  over the daily cap  @clubtreasurer") {
		t.Fatalf("output:\n%s", stdout)
	}
}

func TestDryRun_SaysWhichRulesHaveNoSavedSearch(t *testing.T) {
	t.Parallel()
	b := newBench(t, strings.Replace(watchConfig, `name = "FOMO app"`, `name = "a new rule"`, 1))

	_, stdout, _ := b.run("watch", "--dry-run")

	if strings.Contains(stdout, "@sofia_sol") || !strings.Contains(stdout, "13 posts in the fixture") ||
		!strings.Contains(stdout, "\nno saved search for: a new rule\n") {
		t.Fatalf("output:\n%s", stdout)
	}
}

func TestDryRun_RejectsAFixtureItCannotRead(t *testing.T) {
	t.Parallel()
	cfg := mustParse(t, watchConfig)
	for _, fixture := range []string{"{", `{"group chat investing": {"data": "nope"}}`} {
		var out strings.Builder

		err := marketing.DryRun(&out, cfg, []byte(fixture))

		if errs.CodeOf(err) != errs.CodeDecodeFailed {
			t.Errorf("fixture %q: err = %v, want decode_failed", fixture, err)
		}
	}
}

func TestSelect_FillsTheRoomWithPostsAboveTheMinimum(t *testing.T) {
	t.Parallel()
	c := mustParse(t, minimal)
	scored := []marketing.Scored{
		{Post: post("1", "", 0, 0), Score: 4.9},
		{Post: post("2", "", 0, 0), Score: 8},
		{Post: post("3", "", 0, 0), Score: 6},
		{Post: post("4", "", 0, 0), Score: 7},
	}

	chosen, rest := c.Select(scored, 2)
	if len(chosen) != 2 || chosen[0].Post.ID != "2" || chosen[1].Post.ID != "4" || len(rest) != 2 {
		t.Fatalf("chosen %v, rest %v; want ids 2 and 4 chosen", chosen, rest)
	}
	if chosen, _ := c.Select(scored, -3); len(chosen) != 0 {
		t.Errorf("negative room chose %v", chosen)
	}
	if chosen, _ := c.Select(scored, 10); len(chosen) != 3 {
		t.Errorf("room for all chose %d, want the 3 above the minimum", len(chosen))
	}
}
