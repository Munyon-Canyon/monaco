package marketing_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	searchPath = "/2/tweets/search/recent"
	firstText  = "@sofia_sol · 7800 followers · score 9.97\n\n" +
		"Is the FOMO app just copy trading with extra steps?\n\n" +
		"Angle: Copy trading copies one person.\n\n" +
		"https://x.com/sofia_sol/status/1847000000000014000"
)

func handles(texts []string) []string {
	out := make([]string, len(texts))
	for i, text := range texts {
		out[i], _, _ = strings.Cut(strings.TrimPrefix(text, "@"), " ")
	}
	return out
}

func TestWatch_Once_SendsTheBestPostsFirst(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)

	code, _, stderr := b.run("watch", "--once")

	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	msgs := b.tg.messages()
	want := []string{
		"sofia_sol", "lucia_ahorra", "maria_invests", "clubtreasurer", "amir_codes",
		"nightowl", "defi_dana", "quietmoney", "degen_dave", "pablo_trades",
	}
	if got := handles(msgs); !slices.Equal(got, want) {
		t.Fatalf("alerted authors = %v, want %v", got, want)
	}
	if msgs[0] != firstText {
		t.Errorf("first message =\n%s\nwant\n%s", msgs[0], firstText)
	}
	paths, chats := b.tg.calls()
	if !slices.Equal(chats, slices.Repeat([]string{chatID}, 10)) || paths[0] != "/bot"+botToken+"/sendMessage" {
		t.Errorf("telegram saw chats %v and path %s", chats, paths[0])
	}
}

func TestWatch_Once_LeavesOutWhatIsNotWorthAReply(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)

	b.run("watch", "--once")

	joined := strings.Join(b.tg.messages(), "\n")
	for _, skipped := range []string{"traderjoe", "stocksbot24", "bigfund", "etf_ellie"} {
		if strings.Contains(joined, skipped) {
			t.Errorf("alerted on %s, who is excluded or scores under the minimum", skipped)
		}
	}
}

func TestWatch_Once_LogsEachAlertAsAJSONLine(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)

	b.run("watch", "--once")

	lines := b.alerts()
	if len(lines) != 10 {
		t.Fatalf("%d log lines, want 10", len(lines))
	}
	raw, _ := json.Marshal(lines[0])
	want := `{"angle":"Copy trading copies one person.","at":"2026-10-18T10:05:00Z",` +
		`"post_id":"1847000000000014000","rules":["FOMO app"],"score":9.97}`
	if string(raw) != want {
		t.Errorf("first line = %s, want %s", raw, want)
	}
	if rules := lines[3]["rules"]; !slices.Equal(
		toStrings(rules),
		[]string{"group chat investing", "investment club"},
	) {
		t.Errorf("rules of the post two searches found = %v", rules)
	}
}

func toStrings(v any) []string {
	items := v.([]any)
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.(string)
	}
	return out
}

func TestWatch_Once_AsksXOncePerRuleWithoutWriting(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)

	b.run("watch", "--once")

	reqs := b.x.sent()
	if len(reqs) != 9 {
		t.Fatalf("%d requests to X, want one per rule", len(reqs))
	}
	for _, r := range reqs {
		if r.method != http.MethodGet || r.path != searchPath || r.auth != "Bearer "+xToken {
			t.Errorf("request = %+v, want an authenticated GET of recent search", r)
		}
		if r.query.Get("start_time") != "2026-10-18T09:05:00Z" || r.query.Has("since_id") {
			t.Errorf("first poll query = %v, want one hour of history and no since_id", r.query)
		}
	}
	want := `("group chat" (stocks OR invest OR portfolio)) (lang:en OR lang:es) -is:retweet -is:reply -"giveaway"`
	if got := reqs[0].query.Get("query"); got != want {
		t.Errorf("first query = %s, want %s", got, want)
	}
}

func TestWatch_Once_RemembersCursorsAndCounts(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)

	b.run("watch", "--once")

	st := b.state()
	newest := map[string]string{}
	for name, body := range fixturePages(t) {
		var page struct {
			Meta struct {
				NewestID string `json:"newest_id"`
			} `json:"meta"`
		}
		if err := json.Unmarshal([]byte(body), &page); err != nil {
			t.Fatal(err)
		}
		newest[name] = page.Meta.NewestID
	}
	if len(st.Cursors) != 9 || st.Cursors["FOMO app"] != newest["FOMO app"] ||
		st.Cursors["xStocks"] != newest["xStocks"] {
		t.Errorf("cursors = %v, want each rule's newest id %v", st.Cursors, newest)
	}
	if st.Sent != 10 || st.Spent != 225_000 || st.Day != "2026-10-18" || len(st.Seen) != 14 {
		t.Errorf(
			"state = sent %d, spent %d, day %s, %d seen; want 10, 225000, 2026-10-18, 14",
			st.Sent,
			st.Spent,
			st.Day,
			len(st.Seen),
		)
	}
}

func TestWatch_SecondRunAsksSinceIDAndRepeatsNothing(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)
	b.run("watch", "--once")
	first := b.state()
	b.x.forget()
	b.clock.Advance(5 * time.Minute)

	code, _, stderr := b.run("watch", "--once")

	if code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if got := len(b.tg.messages()); got != 10 {
		t.Errorf("%d messages after two runs, want the first run's 10 only (X returned the same posts again)", got)
	}
	if got := len(b.alerts()); got != 10 {
		t.Errorf("%d alert lines after two runs, want 10", got)
	}
	for _, r := range b.x.sent() {
		if r.query.Has("start_time") || r.query.Get("since_id") == "" {
			t.Errorf("second poll query = %v, want since_id", r.query)
		}
	}
	if b.x.sent()[0].query.Get("since_id") != first.Cursors["group chat investing"] {
		t.Errorf("since_id = %s, want the cursor the first run stored", b.x.sent()[0].query.Get("since_id"))
	}
}

func TestWatch_StopsAtTheDailyCapAndResetsNextDay(t *testing.T) {
	t.Parallel()
	b := newBench(t, strings.Replace(watchConfig, "daily_cap = 30", "daily_cap = 3", 1))
	b.run("watch", "--once")
	if got := handles(b.tg.messages()); !slices.Equal(got, []string{"sofia_sol", "lucia_ahorra", "maria_invests"}) {
		t.Fatalf("first run alerted %v, want the best three", got)
	}

	b.clock.Advance(5 * time.Minute)
	fresh := onePost(
		"1847000000000099000",
		"new_user",
		"Which group chat portfolio app should we use?",
		b.clock.Now().Add(-time.Minute),
	)
	b.x.serve(map[string]string{"group chat investing": fresh}, nil)
	b.run("watch", "--once")
	if got := len(b.tg.messages()); got != 3 {
		t.Fatalf("%d messages after the cap, want 3", got)
	}

	b.clock.Advance(24 * time.Hour)
	next := onePost(
		"1847000000000099500",
		"next_user",
		"Which group chat portfolio app should we use?",
		b.clock.Now().Add(-time.Minute),
	)
	b.x.serve(map[string]string{"group chat investing": next}, nil)
	b.run("watch", "--once")
	if got := handles(b.tg.messages()); len(got) != 4 || got[3] != "next_user" {
		t.Fatalf("authors after the next UTC day = %v, want next_user as the fourth", got)
	}
	if st := b.state(); st.Day != "2026-10-19" || st.Sent != 1 {
		t.Errorf("state = day %s, sent %d; want 2026-10-19 and 1", st.Day, st.Sent)
	}
}

func TestWatch_StopsPollingAtTheSpendBudget(t *testing.T) {
	t.Parallel()
	b := newBench(t, strings.Replace(watchConfig, "daily_spend_usd = 5.0", "daily_spend_usd = 0.2", 1))
	b.run("watch", "--once")
	b.x.forget()
	b.clock.Advance(5 * time.Minute)

	code, _, stderr := b.run("watch", "--once")

	if code != 0 || len(b.x.sent()) != 0 || !strings.Contains(stderr, "daily spend budget used up") {
		t.Fatalf("exit %d, %d X requests, stderr %q; want 0, none and the budget notice", code, len(b.x.sent()), stderr)
	}
	b.clock.Advance(24 * time.Hour)
	b.run("watch", "--once")
	if len(b.x.sent()) != 9 {
		t.Errorf("%d X requests the next day, want polling to resume", len(b.x.sent()))
	}
}

func TestWatch_OneFailingSearchDoesNotStopTheRest(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)
	b.x.serve(fixturePages(t), map[string]int{"xStocks": http.StatusInternalServerError})

	code, _, stderr := b.run("watch", "--once")

	if code != 1 || !strings.Contains(stderr, `"rule":"xStocks"`) || !strings.Contains(stderr, "search failed") {
		t.Fatalf("exit %d, stderr %s; want 1 and the failed rule named", code, stderr)
	}
	if got := len(b.tg.messages()); got != 9 {
		t.Errorf("%d messages, want the 9 from the rules that worked", got)
	}
	if _, ok := b.state().Cursors["xStocks"]; ok {
		t.Error("the failed rule moved its cursor")
	}

	b.x.serve(fixturePages(t), nil)
	b.run("watch", "--once")
	if got := handles(b.tg.messages()); len(got) != 10 || got[9] != "amir_codes" {
		t.Errorf("authors after the retry = %v, want amir_codes added once", got)
	}
}

func TestWatch_TelegramDownLosesNothing(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)
	b.tg.failWith(http.StatusInternalServerError)

	code, _, _ := b.run("watch", "--once")

	st := b.state()
	if code != 1 || st.Sent != 0 || len(st.Cursors) != 0 || len(st.Seen) != 4 {
		t.Fatalf("exit %d, sent %d, %d cursors, %d seen; want 1, 0, 0 and only the 4 skipped posts seen",
			code, st.Sent, len(st.Cursors), len(st.Seen))
	}

	b.tg.failWith(0)
	b.run("watch", "--once")
	if got := len(b.tg.messages()); got != 10 {
		t.Errorf("%d messages once Telegram is back, want all 10", got)
	}
}

func failEveryRule(b *bench, status int) {
	fail := map[string]int{}
	for _, r := range b.x.rules {
		fail[r.Name] = status
	}
	b.x.serve(fixturePages(b.t), fail)
}

func TestWatch_TellsTelegramOnceWhenXKeepsFailing(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)
	failEveryRule(b, http.StatusUnauthorized)

	for run := 1; run <= 4; run++ {
		if code, _, _ := b.run("watch", "--once"); code != 1 {
			t.Fatalf("run %d exit %d, want 1", run, code)
		}
		want := 0
		if run >= 3 {
			want = 1
		}
		if got := len(b.tg.messages()); got != want {
			t.Fatalf("after run %d: %d Telegram messages, want %d", run, got, want)
		}
	}
	if notice := b.tg.messages()[0]; !strings.Contains(notice, "failed 3 polls in a row") ||
		strings.Contains(notice, xToken) {
		t.Errorf("notice = %q", notice)
	}

	b.x.serve(fixturePages(t), nil)
	b.run("watch", "--once")
	if st := b.state(); st.Failures != 0 || st.Sent != 10 {
		t.Errorf("after recovery: failures %d, sent %d; want 0 and 10", st.Failures, st.Sent)
	}
}

func TestWatch_OutageNoticeThatCannotBeSentIsLogged(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)
	failEveryRule(b, http.StatusUnauthorized)
	b.tg.failWith(http.StatusInternalServerError)

	var stderr string
	for range 3 {
		_, _, stderr = b.run("watch", "--once")
	}

	if !strings.Contains(stderr, "outage notice not sent") {
		t.Fatalf("stderr = %s, want the unsent notice logged", stderr)
	}
}

func TestWatch_CancelledMidPollSavesNothing(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)
	ctx, cancel := context.WithCancel(t.Context())
	b.x.cancelOnRequest(cancel)

	code, _, _ := b.runWith(ctx, "watch", "--once")

	if _, err := os.Stat(
		filepath.Join(b.repo, ".build", "marketing", "state.json"),
	); code != 1 || err == nil ||
		len(b.tg.messages()) != 0 {
		t.Fatalf(
			"exit %d, state file err %v, %d messages; want 1, no state file and none",
			code,
			err,
			len(b.tg.messages()),
		)
	}
}

func TestWatch_PollsEveryFiveMinutesUntilStopped(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)
	ticks := make(chan time.Duration, 1)
	b.clock.NotifyTickers(ticks)
	failEveryRule(b, http.StatusUnauthorized)
	ctx, cancel := context.WithCancel(t.Context())
	exit := make(chan int, 1)
	go func() {
		code, _, _ := b.runWith(ctx, "watch")
		exit <- code
	}()

	if every := <-ticks; every != 5*time.Minute {
		t.Fatalf("ticker = %s, want 5m", every)
	}
	if got := len(b.x.sent()); got != 9 {
		t.Fatalf("%d requests before the first tick, want the immediate poll's 9", got)
	}
	b.x.serve(fixturePages(t), nil)
	b.clock.Advance(5 * time.Minute)
	testkit.Eventually(t, func() bool { return len(b.tg.messages()) == 10 }, 10*time.Second)
	cancel()

	if code := <-exit; code != 0 {
		t.Fatalf("exit %d after stopping, want 0", code)
	}
}

func TestWatch_KeepsLoggingToStderrWhenTheLogFileCannotBeWritten(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)
	if err := os.MkdirAll(filepath.Join(b.repo, ".build", "marketing", "watch.log"), 0o750); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := b.run("watch", "--once")

	if code != 0 || !strings.Contains(stderr, `"msg":"poll done"`) {
		t.Fatalf("exit %d, stderr %s; want a clean poll that still logged to stderr", code, stderr)
	}
}

func TestWatch_LogsToTheFileToo(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)

	b.run("watch", "--once")

	log := b.file(".build/marketing/watch.log")
	if !strings.Contains(log, `"msg":"poll done"`) || !strings.Contains(log, `"alerts":10`) ||
		strings.Contains(log, botToken) ||
		strings.Contains(log, xToken) {
		t.Fatalf("watch.log = %s", log)
	}
}

func TestWatch_ClipsALongPostInTheMessage(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)
	long := "Which group chat app? " + strings.Repeat("é", 700)
	b.x.serve(
		map[string]string{
			"group chat investing": onePost("1847000000000099000", "wordy", long, fixtureNow().Add(-time.Minute)),
		},
		nil,
	)

	b.run("watch", "--once")

	msgs := b.tg.messages()
	if len(msgs) != 1 || strings.Count(msgs[0], "é") != 578 || !strings.Contains(msgs[0], "é…\n") {
		t.Fatalf("messages = %q, want the text cut at 600 characters and marked", msgs)
	}
}

func TestWatch_SkipsAPostOlderThanTheRecencyWindow(t *testing.T) {
	t.Parallel()
	b := newBench(t, watchConfig)
	stale := onePost(
		"1847000000000000500",
		"latecomer",
		"Which group chat portfolio app should we use?",
		fixtureNow().Add(-3*time.Hour),
	)
	b.x.serve(map[string]string{"group chat investing": stale}, nil)

	code, _, stderr := b.run("watch", "--once")

	if code != 0 || len(b.tg.messages()) != 0 {
		t.Fatalf(
			"exit %d, %d messages, stderr %s; want 0 and none for a post past the recency window",
			code,
			len(b.tg.messages()),
			stderr,
		)
	}
	if _, seen := b.state().Seen["1847000000000000500"]; !seen {
		t.Error("the stale post was not remembered, so it would be looked at again")
	}
}
