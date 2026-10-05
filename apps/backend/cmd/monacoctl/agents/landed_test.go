package agents

import (
	"strings"
	"testing"
	"time"
)

func TestLanded_readsSquashCommitsOnTheTrunkThenFallsBackToCompare(t *testing.T) {
	t.Parallel()
	closedAt := time.Date(2026, 9, 27, 11, 0, 0, 0, time.UTC)
	commits := list("/commits?sha=fb&since=2026-09-27T10:00:00Z")
	squashes := `[{"commit":{"message":"Queue stacks (#12)\n\nCloses #40"}},{"commit":{"message":"Wider (#120)"}}]`
	for _, tc := range []struct {
		name    string
		pr      closedPR
		compare string
		want    bool
	}{
		{"merged, with no API call", closedPR{Number: 7, State: "MERGED"}, "", true},
		{"open, with no API call", closedPR{Number: 7, State: "OPEN"}, "", false},
		{"closed with its squash commit on the trunk", closedPR{12, "CLOSED", "h12", closedAt}, "", true},
		{"(#120) is not #12's squash", closedPR{12, "CLOSED", "h", closedAt}, `{"status":"diverged"}`, false},
		{"closed with neither a squash nor its head", closedPR{13, "CLOSED", "h", closedAt}, `{"status":"diverged"}`, false},
		{"closed with its head on the trunk", closedPR{13, "CLOSED", "h", closedAt}, `{"status":"behind"}`, true},
		{"closed with no close time", closedPR{Number: 12, State: "CLOSED", Head: "h"}, `{"status":"identical"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			if tc.pr.State == "CLOSED" && !tc.pr.ClosedAt.IsZero() {
				f.hub.on(commits, squashes)
			}
			if strings.HasPrefix(tc.name, "(#120)") {
				f.hub.on(commits, `[{"commit":{"message":"Wider (#120)"}},{"commit":{"message":"Says (#12) inside"}}]`)
			}
			if tc.compare != "" {
				f.hub.on(get("/compare/fb..."+tc.pr.Head), tc.compare)
			}
			got, err := f.Env(t).landed(t.Context(), tc.pr)
			if err != nil || got != tc.want {
				t.Fatalf("landed = %v, %v; want %v", got, err, tc.want)
			}
		})
	}
}

func TestLanded_readsTheTrunkOncePerStackAndAgainForANewerClose(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	env := f.Env(t)
	closedAt := f.now.Add(-time.Hour)
	f.hub.on(
		list("/commits?sha=fb&since=2026-09-27T10:00:00Z"),
		`[{"commit":{"message":"A (#1)"}},{"commit":{"message":"B (#2)"}}]`,
	)
	for _, n := range []int{1, 2} {
		if ok, err := env.landed(
			t.Context(),
			closedPR{n, "CLOSED", "h", closedAt.Add(time.Duration(n-1) * 2 * time.Minute)},
		); !ok ||
			err != nil {
			t.Fatalf("#%d: %v %v", n, ok, err)
		}
		f.hub.on(list("/commits?sha=fb&since=2026-09-27T10:00:00Z"), `{"message":"read again"}`)
	}
	for _, since := range []string{"2026-09-27T09:59:00Z", "2026-09-27T11:01:00Z"} {
		f.hub.on(list("/commits?sha=fb&since="+since), `{"message":"read again"}`)
	}
	for name, pr := range map[string]closedPR{
		"closed before the cached window": {3, "CLOSED", "h", closedAt.Add(-time.Minute)},
		"closed after the trunk was read": {4, "CLOSED", "h", f.now.Add(time.Minute)},
	} {
		if _, err := env.landed(t.Context(), pr); err == nil || !strings.Contains(err.Error(), "list fb commits") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestClosed_readsTheRESTCloseTime(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 27, 11, 0, 0, 0, time.UTC)
	got := PR{Number: 5, State: "closed", ClosedAt: &at, Head: Ref{SHA: "s"}}.closed()
	if got != (closedPR{5, "CLOSED", "s", at}) {
		t.Fatalf("%+v", got)
	}
}
