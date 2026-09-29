package agents

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestStatus_sortsPullsAndCountsTheRemainder(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	sha := strings.Repeat("c", 40)
	const n = 18
	open := make([]PR, n)
	for i := range open {
		num := n - i
		p := headed(num, sha)
		p.Head.Ref = "h" + strconv.Itoa(num)
		p.Base.Ref = "fb-checkpoint-1"
		if num != n {
			p.Base.Ref = "h" + strconv.Itoa(num+1)
		}
		open[i] = p
	}
	f.hub.on(list("/pulls?state=open"), open)
	f.hub.on("GET /repos/o/r/commits/"+sha+"/check-runs?per_page=100", `{"check_runs":[]}`)
	f.hub.on(list("/commits/"+sha+"/statuses?"), []GHStatus{})
	body, err := f.Env(t).statusText(context.Background())
	if err != nil || !strings.Contains(body, "and 1 more") {
		t.Fatal(body, err)
	}
	first, second := strings.Index(body, "| #1 |"), strings.Index(body, "| #2 |")
	if first < 0 || second < 0 || first > second || strings.Contains(body, "| #18 |") {
		t.Fatal(body)
	}
}

func TestWatch_idlesOnlyPastTwentyMinutes(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	started := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	f.owner(t, Record{Ticket: 1, State: Running, Started: started, Worktree: f.dir})
	f.hub.on(get("/issues/1"), Issue{UpdatedAt: started})
	f.hub.on(list("/pulls?state=open"), []PR{})
	f.noFailures()
	stamp := []byte(strconv.FormatInt(started.Unix(), 10) + "\n")
	for _, tc := range []struct {
		now  time.Time
		idle bool
	}{
		{started.Add(idleAfter), false},
		{started.Add(idleAfter + time.Nanosecond), true},
	} {
		env := f.Env(t)
		env.Now = func() time.Time { return tc.now }
		env.Run = func(_ context.Context, _, _, name string, args ...string) ([]byte, error) {
			if name == "git" && len(args) > 0 && args[0] == "log" {
				return stamp, nil
			}
			if name == "git" {
				return []byte("HEAD\n"), nil
			}
			return nil, errors.New("down")
		}
		var buf strings.Builder
		err := watchCmd(context.Background(), env, nil, &buf)
		got := strings.Contains(buf.String(), "idle: #1")
		if got != tc.idle || (tc.idle && err == nil) || (!tc.idle && err != nil) {
			t.Fatalf("after %s idle=%v err=%v %q", tc.now.Sub(started), got, err, buf.String())
		}
	}
}

func TestPushTime_okFollowsTheClockParse(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	env.Run = pushRun("100\n")
	got, ok, err := env.pushTime(context.Background(), f.dir)
	if err != nil || !ok || !got.Equal(time.Unix(100, 0).UTC()) {
		t.Fatalf("ok=%v t=%s err=%v", ok, got, err)
	}
	env.Run = pushRun("nope\n")
	if _, ok, err = env.pushTime(context.Background(), f.dir); err == nil || ok {
		t.Fatalf("bad clock ok=%v err=%v", ok, err)
	}
}

func pushRun(stamp string) Runner {
	return func(_ context.Context, _, _, name string, args ...string) ([]byte, error) {
		if name == "git" && len(args) > 1 && args[1] == "--abbrev-ref" {
			return []byte("topic\n"), nil
		}
		if name == "git" && len(args) > 0 && args[0] == "log" {
			return []byte(stamp), nil
		}
		return nil, errors.New("down")
	}
}
