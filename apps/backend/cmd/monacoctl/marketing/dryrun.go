package marketing

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/providers/x"
)

const (
	fixturePath = "testdata/search.json"
	fixtureText = 120
)

//go:embed testdata/search.json
var fixtures embed.FS

func Fixture() []byte {
	raw, _ := fixtures.ReadFile(fixturePath)
	return raw
}

func parseFixture(raw []byte) (map[string]x.Page, error) {
	var byRule map[string]json.RawMessage
	if err := json.Unmarshal(raw, &byRule); err != nil {
		return nil, errs.Wrap(err, errs.CodeDecodeFailed, "marketing.parseFixture")
	}
	pages := make(map[string]x.Page, len(byRule))
	for name, body := range byRule {
		page, err := x.Decode(body)
		if err != nil {
			return nil, err
		}
		pages[name] = page
	}
	return pages, nil
}

func DryRun(w io.Writer, c Config, fixture []byte) error {
	pages, err := parseFixture(fixture)
	if err != nil {
		return err
	}
	hits := map[string]*Hit{}
	var unsaved []string
	for _, r := range c.Rules {
		page, saved := pages[r.Name]
		if !saved {
			unsaved = append(unsaved, r.Name)
		}
		addPage(hits, r, page)
	}
	var newest time.Time
	for _, h := range hits {
		if h.Post.CreatedAt.After(newest) {
			newest = h.Post.CreatedAt
		}
	}
	now := newest.Add(time.Minute)
	var scored []Scored
	var skipped []string
	for _, id := range slices.Sorted(maps.Keys(hits)) {
		h := hits[id]
		if why := c.Skipped(h.Post, now); why != "" {
			skipped = append(skipped, fmt.Sprintf("\n  skip  %s  @%s  %s\n", why, h.Post.Handle, h.Post.URL()))
			continue
		}
		scored = append(scored, c.Score(*h, now))
	}
	chosen, rest := c.Select(scored, c.DailyCap)
	_, _ = fmt.Fprintf(
		w,
		"dry run: %d posts in the fixture, scored as of %s; %d would alert (min score %v, cap %d a day)\n",
		len(hits),
		now.Format(time.RFC3339),
		len(chosen),
		c.MinScore,
		c.DailyCap,
	)
	if len(unsaved) > 0 {
		_, _ = fmt.Fprintf(w, "no saved search for: %s\n", strings.Join(unsaved, ", "))
	}
	for _, s := range chosen {
		_, _ = io.WriteString(w, block(s, "alert"))
	}
	for _, s := range rest {
		verdict := "over the daily cap"
		if s.Score < c.MinScore {
			verdict = "below min score"
		}
		_, _ = io.WriteString(w, block(s, verdict))
	}
	_, _ = io.WriteString(w, strings.Join(skipped, ""))
	return nil
}

func block(s Scored, verdict string) string {
	p := s.Post
	text := clip(strings.Join(strings.Fields(p.Text), " "), fixtureText)
	return fmt.Sprintf("\n%6.2f  %s  @%s · %d followers · %s\n        %s\n        angle: %s\n        %s\n",
		s.Score, verdict, p.Handle, p.Followers, strings.Join(s.Rules, ", "), text, s.Angle, p.URL())
}
