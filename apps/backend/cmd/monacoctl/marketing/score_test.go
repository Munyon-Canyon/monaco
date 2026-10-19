package marketing_test

import (
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/cmd/monacoctl/marketing"
	"github.com/monaco/monaco/apps/backend/internal/providers/x"
)

func noon() time.Time { return time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC) }

func post(id, text string, followers int, age time.Duration) x.Post {
	return x.Post{ID: id, Text: text, Handle: "maria", Followers: followers, CreatedAt: noon().Add(-age)}
}

func TestScore_AddsEachSignal(t *testing.T) {
	t.Parallel()
	c := mustParse(t, minimal)
	rule := c.Rules[0]
	tests := []struct {
		name string
		post x.Post
		want float64
	}{
		{"everything at once", post("1", "Which app is this?", 5000, 0), 9.8},
		{"half the recency window gone", post("1", "Which app is this?", 5000, time.Hour), 8.3},
		{"older than the window", post("1", "Which app is this?", 5000, 3*time.Hour), 6.8},
		{"posted after the clock", post("1", "Which app is this?", 5000, -time.Hour), 9.8},
		{"no question mark", post("1", "Which app is this", 5000, 0), 8.3},
		{"below the band", post("1", "Which app is this?", 499, 0), 7.8},
		{"band floor counts", post("1", "Which app is this?", 500, 0), 9.8},
		{"band ceiling counts", post("1", "Which app is this?", 200000, 0), 9.8},
		{"above the band", post("1", "Which app is this?", 200001, 0), 7.8},
	}
	for _, tt := range tests {
		got := c.Score(marketing.Hit{Post: tt.post, Rules: []marketing.Rule{rule}}, noon())
		if got.Score != tt.want {
			t.Errorf("%s: score = %v, want %v", tt.name, got.Score, tt.want)
		}
	}
}

func TestScore_CountsAnOutboundLinkAgainstThePost(t *testing.T) {
	t.Parallel()
	c := mustParse(t, minimal)
	p := post("1", "Which app is this?", 5000, 0)
	p.Link = true

	got := c.Score(marketing.Hit{Post: p, Rules: c.Rules}, noon())

	if got.Score != 8.3 {
		t.Fatalf("score = %v, want 8.3", got.Score)
	}
}

func TestScore_UsesTheHeaviestMatchingRule(t *testing.T) {
	t.Parallel()
	c := mustParse(t, minimal)
	rules := []marketing.Rule{{Name: "light", Weight: 0.25}, {Name: "heavy", Weight: 0.75}}

	got := c.Score(marketing.Hit{Post: post("1", "no question", 5000, 0), Rules: rules}, noon())

	if got.Score != 8 {
		t.Errorf("score = %v, want 8", got.Score)
	}
	if len(got.Rules) != 2 || got.Rules[0] != "light" || got.Rules[1] != "heavy" {
		t.Errorf("rules = %v, want both names in order", got.Rules)
	}
}

func TestScore_RoundsToTwoDecimals(t *testing.T) {
	t.Parallel()
	c := mustParse(t, minimal)
	c.Weights = marketing.Weights{Recency: 1, RecencyMinutes: 120, FollowerMin: 1, FollowerMax: 2}

	got := c.Score(marketing.Hit{Post: post("1", "x", 0, 40*time.Minute), Rules: c.Rules}, noon())

	if got.Score != 0.67 {
		t.Fatalf("score = %v, want 0.67", got.Score)
	}
}

func TestScore_CarriesTheAngleForThePostText(t *testing.T) {
	t.Parallel()
	c := mustParse(t, minimal)

	copying := c.Score(marketing.Hit{Post: post("1", "Is this copy trading?", 5000, 0), Rules: c.Rules}, noon())
	plain := c.Score(marketing.Hit{Post: post("2", "Which app is this?", 5000, 0), Rules: c.Rules}, noon())

	if copying.Angle != "Copy trading copies one person." || plain.Angle != "Read the thread first." {
		t.Fatalf("angles = %q and %q", copying.Angle, plain.Angle)
	}
}

func TestExcluded_NamesWhyAPostIsSkipped(t *testing.T) {
	t.Parallel()
	c := mustParse(t, minimal)
	tests := []struct {
		name string
		post x.Post
		want string
	}{
		{"a normal post", post("1", "Which app is this?", 5000, 0), ""},
		{"one follower short of the floor", post("1", "Which app is this?", 199, 0), "under the follower floor"},
		{"exactly at the floor", post("1", "Which app is this?", 200, 0), ""},
		{"cashtags only", post("1", "$TSLA $AAPL $nvda", 5000, 0), "cashtags only"},
		{"cashtags and a link", post("1", "$TSLA https://t.co/abc", 5000, 0), "cashtags only"},
		{"cashtags and an emoji", post("1", "🚀 $BTC.X 🚀", 5000, 0), "cashtags only"},
		{"a cashtag in a sentence", post("1", "Bought $TSLA today", 5000, 0), ""},
		{"a cashtag in another language", post("1", "$TSLA ¿alguien más?", 5000, 0), ""},
		{"no cashtag and no words", post("1", "🚀🚀", 5000, 0), ""},
	}
	for _, tt := range tests {
		if got := c.Excluded(tt.post); got != tt.want {
			t.Errorf("%s: Excluded = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestExcluded_KeepsCashtagPostsWhenTheSwitchIsOff(t *testing.T) {
	t.Parallel()
	c := mustParse(t, minimal)
	c.Exclude.CashtagOnly = false

	if got := c.Excluded(post("1", "$TSLA $AAPL", 5000, 0)); got != "" {
		t.Fatalf("Excluded = %q, want the post kept", got)
	}
}

func TestRank_OrdersByScoreThenFreshnessThenID(t *testing.T) {
	t.Parallel()
	at := func(id string, score float64, age time.Duration) marketing.Scored {
		return marketing.Scored{Post: post(id, "", 0, age), Score: score}
	}
	scored := []marketing.Scored{
		at("1", 5, time.Hour), at("2", 5, time.Minute), at("3", 7, 2*time.Hour), at("4", 5, time.Minute),
	}

	marketing.Rank(scored)

	ids := make([]string, len(scored))
	for i, s := range scored {
		ids[i] = s.Post.ID
	}
	if order := strings.Join(ids, ""); order != "3421" {
		t.Fatalf("order = %s, want 3421 (best first, then newest, then highest id)", order)
	}
}
