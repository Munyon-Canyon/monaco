package marketing_test

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/monaco/monaco/apps/backend/cmd/monacoctl/marketing"
	"github.com/monaco/monaco/apps/backend/internal/errs"
)

func TestLoad_ReadsTheStarterFile(t *testing.T) {
	t.Parallel()

	c, err := marketing.Load(os.DirFS("../../../../.."))
	if err != nil {
		t.Fatal(err)
	}

	if c.DailyCap != 30 || strings.Join(c.Languages, ",") != "en,es" || len(c.Rules) != 9 || len(c.Angles) != 8 {
		t.Fatalf(
			"starter config = cap %d, languages %v, %d rules, %d angles",
			c.DailyCap,
			c.Languages,
			len(c.Rules),
			len(c.Angles),
		)
	}
	want := `("group chat" (stocks OR invest OR portfolio)) (lang:en OR lang:es) -is:retweet -is:reply ` +
		`-"giveaway" -"giveaways"`
	if got := c.Query(c.Rules[0]); got != want {
		t.Errorf("first query = %s, want %s", got, want)
	}
	if c.Weights.FollowerMin != 500 || c.Weights.FollowerMax != 200000 {
		t.Errorf("follower band = %d to %d, want 500 to 200000", c.Weights.FollowerMin, c.Weights.FollowerMax)
	}
}

func TestLoad_ReportsAMissingFile(t *testing.T) {
	t.Parallel()

	_, err := marketing.Load(fstest.MapFS{})

	if errs.CodeOf(err) != errs.CodeInvalidConfig {
		t.Fatalf("err = %v, want invalid_config", err)
	}
}

func TestParse_AcceptsTheMinimalConfig(t *testing.T) {
	t.Parallel()

	c := mustParse(t, minimal)

	if c.Rules[0].Name != "club" || c.Weights.RecencyMinutes != 120 || !c.Exclude.CashtagOnly {
		t.Fatalf("config = %+v", c)
	}
}

func TestParse_RejectsWhatCannotRun(t *testing.T) {
	t.Parallel()
	const secondRule = "\n[[rule]]\nname = \"club\"\nquery = 'x'\nweight = 1\n"
	tests := []struct {
		name string
		text string
		want string
	}{
		{"no daily cap", edit("daily_cap = 30", "daily_cap = 0"), "daily_cap must be positive"},
		{"no spend budget", edit("daily_spend_usd = 5.0", "daily_spend_usd = 0"), "daily_spend_usd must be positive"},
		{"negative min score", edit("min_score = 5.0", "min_score = -1.0"), "min_score must not be negative"},
		{"no languages", edit(`["en", "es"]`, "[]"), "languages needs at least one code"},
		{"a language name", edit(`["en", "es"]`, `["english"]`), `language "english" is not a language code`},
		{"no default angle", edit(`"Read the thread first."`, `""`), "default_angle is required"},
		{
			"negative follower floor",
			edit("min_followers = 200", "min_followers = -1"),
			"min_followers must not be negative",
		},
		{"negative weight", edit("keyword = 2.0", "keyword = -0.5"), "score weights must not be negative"},
		{"no recency window", edit("recency_minutes = 120", "recency_minutes = 0"), "recency_minutes must be positive"},
		{"no band floor", edit("follower_min = 500", "follower_min = 0"), "follower_min must be positive"},
		{"inverted band", edit("follower_max = 200000", "follower_max = 400"), "below follower_max"},
		{"a term with quotes", edit(`"free money"`, `"say \"x\""`), "without quotes"},
		{"a blank term", edit(`"free money"`, `"  "`), "without quotes"},
		{
			"no rules",
			edit("[[rule]]\nname = \"club\"\nquery = '\"investment club\"'\nweight = 0.9\n", ""),
			"at least one [[rule]]",
		},
		{"an unnamed rule", edit(`name = "club"`, `name = ""`), "every rule needs a name"},
		{"a repeated rule", minimal + secondRule, `rule "club" appears twice`},
		{"a blank query", edit(`query = '"investment club"'`, `query = ' '`), `rule "club" needs a query`},
		{"a zero weight", edit("weight = 0.9", "weight = 0"), "weight must be above 0 and at most 1"},
		{"a weight over one", edit("weight = 0.9", "weight = 1.5"), "weight must be above 0 and at most 1"},
		{
			"a query X would refuse",
			edit(`'"investment club"'`, "'"+strings.Repeat("a", 600)+"'"),
			"over 512 characters",
		},
		{"an angle without keywords", edit(`["copy trading", "etoro"]`, "[]"), "angle 1 needs keywords"},
		{"an angle with a blank keyword", edit(`"etoro"`, `" "`), "angle 1 has an empty keyword"},
		{"an angle without text", edit(`"Copy trading copies one person."`, `""`), "angle 1 needs text"},
		{"a key nobody reads", "typo = 1\n" + minimal, `unknown key "typo"`},
		{"a nested key nobody reads", edit("keyword = 2.0", "keyword = 2.0\nboost = 1"), `unknown key "score.boost"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := marketing.Parse([]byte(tt.text))

			if errs.CodeOf(err) != errs.CodeInvalidConfig || !strings.Contains(detail(err), tt.want) {
				t.Fatalf("err = %v, want invalid_config containing %q", err, tt.want)
			}
		})
	}
}

func TestParse_RejectsTOMLItCannotRead(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		text string
		want string
	}{
		{"a missing value", edit("daily_cap = 30", "daily_cap ="), "line 2: expected a value"},
		{"a word that is not a value", edit("daily_cap = 30", "daily_cap = yes"), "expected a value, got yes"},
		{"a missing equals sign", edit("daily_cap = 30", "daily_cap 30"), "expected = after daily_cap"},
		{"a missing key", edit("daily_cap = 30", "= 30"), "expected a key"},
		{"a quoted key", edit("daily_cap = 30", `"daily_cap" = 30`), "expected a key"},
		{"an unterminated string at the end of the file", minimal + "\nx = \"abc", "unterminated string"},
		{
			"a string cut by the line end",
			edit(`default_angle = "Read the thread first."`, "default_angle = \"Read\nthe thread\""),
			"a string ends on its line",
		},
		{"a string with a bad escape", edit(`"Read the thread first."`, `"Read \q"`), "a string has a bad escape"},
		{
			"an unterminated literal string",
			edit("query = '\"investment club\"'", "query = 'club"),
			"unterminated string",
		},
		{
			"a literal string cut by the line end",
			edit("query = '\"investment club\"'", "query = 'club\n'"),
			"unterminated string",
		},
		{"an unterminated array at the end of the file", minimal + "\nx = [\"en\"", "unterminated array"},
		{"an array that ends after a comma", minimal + "\nx = [\"en\",", "unterminated array"},
		{
			"an array that swallows the next key",
			edit(`["en", "es"]`, `["en", "es"`),
			"expected , or ] in an array",
		},
		{"an array item that is no value", edit(`["en", "es"]`, `["en", @]`), "expected a value"},
		{"an array of numbers", edit(`["en", "es"]`, `[1, 2]`), "arrays hold strings only"},
		{"an array in an array", edit(`["en", "es"]`, `[["en"]]`), "arrays hold strings only"},
		{"an array missing a comma", edit(`["en", "es"]`, `["en" "es"]`), "expected , or ] in an array"},
		{"text after a value", edit("daily_cap = 30", "daily_cap = 30 extra"), "unexpected text after a value"},
		{"a number that is not whole", edit("daily_cap = 30", "daily_cap = 3.5"), "daily_cap must be a whole number"},
		{"a number with a letter", edit("daily_cap = 30", "daily_cap = 3x"), "daily_cap must be a whole number"},
		{
			"a string where a whole number goes",
			edit("daily_cap = 30", `daily_cap = "30"`),
			"daily_cap must be a whole number",
		},
		{"a string where a number goes", edit("min_score = 5.0", `min_score = "5"`), "min_score must be a number"},
		{"a word where a number goes", edit("min_score = 5.0", "min_score = true"), "min_score must be a number"},
		{"a string where a list goes", edit(`["en", "es"]`, `"en"`), "languages must be an array of strings"},
		{
			"a string where a switch goes",
			edit("retweets = true", `retweets = "yes"`),
			"exclude.retweets must be true or false",
		},
		{"a number where a string goes", edit(`name = "club"`, "name = 7"), "rule.name must be a string"},
		{"a key set twice", edit("daily_cap = 30", "daily_cap = 30\ndaily_cap = 31"), "key daily_cap appears twice"},
		{"a table set twice", minimal + "\n[exclude]\nretweets = false\n", "table [exclude] appears twice"},
		{"a table header left open", edit("[exclude]", "[exclude"), "a table header is [name] or [[name]]"},
		{"a table header with no name", edit("[exclude]", "[]"), "a table header is [name] or [[name]]"},
		{"a dotted table header", edit("[exclude]", "[exclude.more]"), "a table header is [name] or [[name]]"},
		{"a list header left open", edit("[[angle]]", "[[angle]"), "a table header is [name] or [[name]]"},
		{"a table nobody reads", minimal + "\n[mystery]\nx = 1\n", `unknown table "mystery"`},
		{"a list of tables nobody reads", minimal + "\n[[mystery]]\nx = 1\n", `unknown table "mystery"`},
		{"a key nobody reads in a rule", edit("weight = 0.9", "weight = 0.9\ncolor = 1"), `unknown key "rule.color"`},
		{
			"a key nobody reads in an angle",
			edit(`text = "Copy trading copies one person."`, "text = \"Copy.\"\nbonus = 1"),
			`unknown key "angle.bonus"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := marketing.Parse([]byte(tt.text))

			if errs.CodeOf(err) != errs.CodeInvalidConfig || !strings.Contains(detail(err), tt.want) {
				t.Fatalf("err = %v, want invalid_config containing %q", err, tt.want)
			}
		})
	}
}

func TestParse_ReadsTOMLWrittenInOtherStyles(t *testing.T) {
	t.Parallel()
	text := "# a leading comment\r\n" +
		"daily_cap = 3_0  # underscores and a trailing comment\r\n" +
		"daily_spend_usd = 5\r\n" +
		"min_score = +5.0\r\n" +
		"languages = [\n  \"en\", # the first\n  'es',\n]\r\n" +
		"default_angle = \"She said \\\"hi\\\" \\u00e9\"\r\n" +
		"[exclude]\r\nretweets = false\r\nterms = []\r\n" +
		"[score]\r\nrecency = 3\r\nfollowers = 2\r\nquestion = 1.5\r\nno_link = 1.5\r\nkeyword = 2\r\n" +
		"recency_minutes = 120\r\nfollower_min = 500\r\nfollower_max = 200_000\r\n" +
		"[[rule]]\r\nname = 'a \"quoted\" rule'\r\nquery = 'x'\r\nweight = 1\r\n" +
		"[[rule]]\r\nname = \"second\"\r\nquery = 'y'\r\nweight = 0.5\r\n"

	c := mustParse(t, text)

	type shape struct {
		dailyCap, followerMax, terms, rules, angles int
		spend, minScore, secondWeight               float64
		languages, defaultAngle, firstRule          string
		retweets                                    bool
	}
	got := shape{
		c.DailyCap, c.Weights.FollowerMax, len(c.Exclude.Terms), len(c.Rules), len(c.Angles),
		c.DailySpend, c.MinScore, c.Rules[1].Weight,
		strings.Join(c.Languages, ","), c.DefaultAngle, c.Rules[0].Name,
		c.Exclude.Retweets,
	}
	want := shape{
		30, 200000, 0, 2, 0,
		5, 5, 0.5,
		"en,es", `She said "hi" é`, `a "quoted" rule`,
		false,
	}
	if got != want {
		t.Fatalf("config = %+v, want %+v", got, want)
	}
}

func TestQuery_JoinsRuleLanguagesAndExclusions(t *testing.T) {
	t.Parallel()
	c := mustParse(t, minimal)

	want := `("investment club") (lang:en OR lang:es) -is:retweet -is:reply -"giveaway" -"free money"`
	if got := c.Query(c.Rules[0]); got != want {
		t.Fatalf("query = %s, want %s", got, want)
	}

	c.Exclude.Retweets, c.Exclude.Replies, c.Exclude.Terms, c.Languages = false, false, nil, []string{"es"}
	if got := c.Query(c.Rules[0]); got != `("investment club") (lang:es)` {
		t.Fatalf("query without exclusions = %s", got)
	}
}

func TestAngle_FirstMatchingEntryWins(t *testing.T) {
	t.Parallel()
	const twoAngles = "\n[[angle]]\nkeywords = [\"copy\", \"nvda\"]\ntext = \"Second.\"\n"
	c := mustParse(t, minimal+twoAngles)
	const first, second, none = "Copy trading copies one person.", "Second.", "Read the thread first."
	tests := []struct {
		text string
		want string
	}{
		{"So is this just copy trading?", first},
		{"COPY TRADING again", first},
		{"Tried eToro last year", first},
		{"copy trading, etoro, whatever", first},
		{"nvda is up", second},
		{"my copy of the book", second},
		{"petoro is not a word here", none},
		{"copy-trading with a hyphen", second},
		{"unrelated", none},
		{"", none},
	}
	for _, tt := range tests {
		if got := c.Angle(tt.text); got != tt.want {
			t.Errorf("Angle(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}

func TestAngle_MatchesKeywordsNextToAccentedLetters(t *testing.T) {
	t.Parallel()
	c := mustParse(t, edit(`["copy trading", "etoro"]`, `["pérdida"]`))

	if got := c.Angle("Una pérdida grande"); got != "Copy trading copies one person." {
		t.Errorf("accented keyword not matched: %q", got)
	}
	if got := c.Angle("lapérdida"); got != "Read the thread first." {
		t.Errorf("matched inside a word: %q", got)
	}
}

func TestBudget_TurnsDollarsIntoMicros(t *testing.T) {
	t.Parallel()
	c := mustParse(t, edit("daily_spend_usd = 5.0", "daily_spend_usd = 0.225"))

	if got := c.Budget(); got != 225_000 {
		t.Fatalf("Budget = %d, want 225000", got)
	}
}

func TestParse_LeavesOutWhatTheFileDoesNotSay(t *testing.T) {
	t.Parallel()
	const exclude = "[exclude]\nretweets = true\nreplies = true\nterms = [\"giveaway\", \"free money\"]\n" +
		"cashtag_only = true\nmin_followers = 200\n"
	text := strings.Replace(edit("min_score = 5.0", ""), exclude, "", 1)

	c := mustParse(t, text)

	if c.MinScore != 0 || c.Exclude.Retweets || c.Exclude.MinFollowers != 0 || len(c.Exclude.Terms) != 0 {
		t.Fatalf("config = %+v, want zero values for the keys the file leaves out", c)
	}
}
