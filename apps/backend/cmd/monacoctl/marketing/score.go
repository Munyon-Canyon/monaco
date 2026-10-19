package marketing

import (
	"cmp"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/monaco/monaco/apps/backend/internal/providers/x"
)

var (
	cashtag = regexp.MustCompile(`\$[A-Za-z][A-Za-z0-9.]{0,9}`)
	link    = regexp.MustCompile(`https?://\S+`)
)

type Hit struct {
	Post  x.Post
	Rules []Rule
}

type Scored struct {
	Post  x.Post
	Rules []string
	Score float64
	Angle string
}

func (c Config) Excluded(p x.Post) string {
	switch {
	case p.Followers < c.Exclude.MinFollowers:
		return "under the follower floor"
	case c.Exclude.CashtagOnly && cashtagOnly(p.Text):
		return "cashtags only"
	}
	return ""
}

func (c Config) Score(h Hit, now time.Time) Scored {
	w := c.Weights
	window := float64(time.Duration(w.RecencyMinutes) * time.Minute)
	fresh := min(max(1-float64(now.Sub(h.Post.CreatedAt))/window, 0), 1)
	inBand := w.FollowerMin <= h.Post.Followers && h.Post.Followers <= w.FollowerMax
	names := make([]string, len(h.Rules))
	heaviest := 0.0
	for i, r := range h.Rules {
		names[i] = r.Name
		heaviest = max(heaviest, r.Weight)
	}
	total := w.Recency*fresh +
		w.Followers*flag(inBand) +
		w.Question*flag(strings.Contains(h.Post.Text, "?")) +
		w.NoLink*flag(!h.Post.Link) +
		w.Keyword*heaviest
	return Scored{
		Post: h.Post, Rules: names, Score: math.Round(total*100) / 100, Angle: c.Angle(h.Post.Text),
	}
}

func Rank(scored []Scored) {
	slices.SortFunc(scored, func(a, b Scored) int {
		return cmp.Or(
			cmp.Compare(b.Score, a.Score),
			b.Post.CreatedAt.Compare(a.Post.CreatedAt),
			cmp.Compare(b.Post.ID, a.Post.ID),
		)
	})
}

func flag(on bool) float64 {
	if on {
		return 1
	}
	return 0
}

func cashtagOnly(text string) bool {
	if !cashtag.MatchString(text) {
		return false
	}
	rest := link.ReplaceAllString(cashtag.ReplaceAllString(text, ""), "")
	return !strings.ContainsFunc(rest, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) })
}
