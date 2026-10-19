package marketing

import (
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"regexp"
	"slices"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const (
	ConfigPath      = ".monaco/marketing.toml"
	maxQueryLen     = 512
	microsPerDollar = 1_000_000
)

var languageCode = regexp.MustCompile(`^[a-z]{2,3}$`)

type Config struct {
	DailyCap     int
	DailySpend   float64
	MinScore     float64
	Languages    []string
	DefaultAngle string
	Exclude      Exclude
	Weights      Weights
	Rules        []Rule
	Angles       []Angle
}

type Exclude struct {
	Retweets     bool
	Replies      bool
	Terms        []string
	CashtagOnly  bool
	MinFollowers int
}

type Weights struct {
	Recency        float64
	Followers      float64
	Question       float64
	NoLink         float64
	Keyword        float64
	RecencyMinutes int
	FollowerMin    int
	FollowerMax    int
}

type Rule struct {
	Name   string
	Query  string
	Weight float64
}

type Angle struct {
	Keywords []string
	Text     string
	matcher  *regexp.Regexp
}

func Load(fsys fs.FS) (Config, error) {
	raw, err := fs.ReadFile(fsys, ConfigPath)
	if err != nil {
		return Config{}, errs.Wrap(err, errs.CodeInvalidConfig, "marketing.Load", slog.String("path", ConfigPath))
	}
	return Parse(raw)
}

func Parse(raw []byte) (Config, error) {
	d, err := parseDoc(string(raw))
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := c.fill(d); err != nil {
		return Config{}, err
	}
	if err := c.prepare(); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c *Config) fill(d *doc) error {
	root, exclude, score := d.root, d.table("exclude"), d.table("score")
	root.integer("daily_cap", &c.DailyCap)
	root.float("daily_spend_usd", &c.DailySpend)
	root.float("min_score", &c.MinScore)
	root.strings("languages", &c.Languages)
	root.str("default_angle", &c.DefaultAngle)
	c.fillExclude(exclude)
	c.fillWeights(score)
	rules, angles := d.list("rule"), d.list("angle")
	c.fillRules(rules)
	c.fillAngles(angles)
	tables := slices.Concat([]*table{root, exclude, score}, rules, angles)
	for _, t := range tables {
		if err := t.done(); err != nil {
			return err
		}
	}
	return d.unread()
}

func (c *Config) fillExclude(t *table) {
	t.boolean("retweets", &c.Exclude.Retweets)
	t.boolean("replies", &c.Exclude.Replies)
	t.strings("terms", &c.Exclude.Terms)
	t.boolean("cashtag_only", &c.Exclude.CashtagOnly)
	t.integer("min_followers", &c.Exclude.MinFollowers)
}

func (c *Config) fillWeights(t *table) {
	w := &c.Weights
	t.float("recency", &w.Recency)
	t.float("followers", &w.Followers)
	t.float("question", &w.Question)
	t.float("no_link", &w.NoLink)
	t.float("keyword", &w.Keyword)
	t.integer("recency_minutes", &w.RecencyMinutes)
	t.integer("follower_min", &w.FollowerMin)
	t.integer("follower_max", &w.FollowerMax)
}

func (c *Config) fillRules(tables []*table) {
	for _, t := range tables {
		var r Rule
		t.str("name", &r.Name)
		t.str("query", &r.Query)
		t.float("weight", &r.Weight)
		c.Rules = append(c.Rules, r)
	}
}

func (c *Config) fillAngles(tables []*table) {
	for _, t := range tables {
		var a Angle
		t.strings("keywords", &a.Keywords)
		t.str("text", &a.Text)
		c.Angles = append(c.Angles, a)
	}
}

func (c Config) Query(r Rule) string {
	langs := make([]string, len(c.Languages))
	for i, l := range c.Languages {
		langs[i] = "lang:" + l
	}
	parts := []string{"(" + r.Query + ")", "(" + strings.Join(langs, " OR ") + ")"}
	if c.Exclude.Retweets {
		parts = append(parts, "-is:retweet")
	}
	if c.Exclude.Replies {
		parts = append(parts, "-is:reply")
	}
	for _, term := range c.Exclude.Terms {
		parts = append(parts, `-"`+term+`"`)
	}
	return strings.Join(parts, " ")
}

func (c Config) Budget() int { return int(math.Round(c.DailySpend * microsPerDollar)) }

func (c Config) Angle(text string) string {
	for _, a := range c.Angles {
		if a.matcher.MatchString(text) {
			return a.Text
		}
	}
	return c.DefaultAngle
}

type check struct {
	ok  bool
	msg string
}

func firstFailed(checks ...check) error {
	for _, ch := range checks {
		if !ch.ok {
			return invalid(ch.msg)
		}
	}
	return nil
}

func invalid(detail string) error {
	return errs.New(errs.CodeInvalidConfig, "marketing.config", slog.String("detail", ConfigPath+": "+detail))
}

func (c *Config) prepare() error {
	w := c.Weights
	basic := []check{
		{c.DailyCap > 0, "daily_cap must be positive"},
		{c.DailySpend > 0, "daily_spend_usd must be positive"},
		{c.MinScore >= 0, "min_score must not be negative"},
		{len(c.Languages) > 0, "languages needs at least one code"},
		{c.DefaultAngle != "", "default_angle is required"},
		{c.Exclude.MinFollowers >= 0, "exclude.min_followers must not be negative"},
		{min(w.Recency, w.Followers, w.Question, w.NoLink, w.Keyword) >= 0, "score weights must not be negative"},
		{w.RecencyMinutes > 0, "score.recency_minutes must be positive"},
		{
			0 < w.FollowerMin && w.FollowerMin < w.FollowerMax,
			"score.follower_min must be positive and below follower_max",
		},
		{len(c.Rules) > 0, "at least one [[rule]] is required"},
	}
	languages := make([]check, len(c.Languages))
	for i, l := range c.Languages {
		languages[i] = check{languageCode.MatchString(l), fmt.Sprintf("language %q is not a language code", l)}
	}
	terms := make([]check, len(c.Exclude.Terms))
	for i, term := range c.Exclude.Terms {
		ok := !blank(term) && !strings.Contains(term, `"`)
		terms[i] = check{ok, fmt.Sprintf("exclude term %q must be a word or phrase without quotes", term)}
	}
	if err := firstFailed(slices.Concat(basic, languages, terms)...); err != nil {
		return err
	}
	if err := c.prepareRules(); err != nil {
		return err
	}
	return c.prepareAngles()
}

func (c *Config) prepareRules() error {
	names := make(map[string]bool, len(c.Rules))
	for _, r := range c.Rules {
		if err := firstFailed(
			check{r.Name != "", "every rule needs a name"},
			check{!names[r.Name], fmt.Sprintf("rule %q appears twice", r.Name)},
			check{!blank(r.Query), fmt.Sprintf("rule %q needs a query", r.Name)},
			check{0 < r.Weight && r.Weight <= 1, fmt.Sprintf("rule %q weight must be above 0 and at most 1", r.Name)},
			check{
				len(c.Query(r)) <= maxQueryLen,
				fmt.Sprintf("rule %q makes a search query over %d characters", r.Name, maxQueryLen),
			},
		); err != nil {
			return err
		}
		names[r.Name] = true
	}
	return nil
}

func (c *Config) prepareAngles() error {
	for i, a := range c.Angles {
		if err := firstFailed(
			check{len(a.Keywords) > 0, fmt.Sprintf("angle %d needs keywords", i+1)},
			check{!slices.ContainsFunc(a.Keywords, blank), fmt.Sprintf("angle %d has an empty keyword", i+1)},
			check{a.Text != "", fmt.Sprintf("angle %d needs text", i+1)},
		); err != nil {
			return err
		}
		quoted := make([]string, len(a.Keywords))
		for j, k := range a.Keywords {
			quoted[j] = regexp.QuoteMeta(strings.TrimSpace(k))
		}
		c.Angles[i].matcher = regexp.MustCompile(
			`(?i)(?:^|[^\p{L}\p{N}])(?:` + strings.Join(quoted, "|") + `)(?:$|[^\p{L}\p{N}])`)
	}
	return nil
}

func blank(s string) bool { return strings.TrimSpace(s) == "" }
