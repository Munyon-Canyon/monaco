package marketing_test

import (
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/cmd/monacoctl/marketing"
	"github.com/monaco/monaco/apps/backend/internal/errs"
)

const minimal = `
daily_cap = 30
daily_spend_usd = 5.0
min_score = 5.0
languages = ["en", "es"]
default_angle = "Read the thread first."

[exclude]
retweets = true
replies = true
terms = ["giveaway", "free money"]
cashtag_only = true
min_followers = 200

[score]
recency = 3.0
followers = 2.0
question = 1.5
no_link = 1.5
keyword = 2.0
recency_minutes = 120
follower_min = 500
follower_max = 200000

[[rule]]
name = "club"
query = '"investment club"'
weight = 0.9

[[angle]]
keywords = ["copy trading", "etoro"]
text = "Copy trading copies one person."
`

func mustParse(t *testing.T, text string) marketing.Config {
	t.Helper()
	c, err := marketing.Parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func detail(err error) string {
	for _, a := range errs.Detail(err) {
		if a.Key == "detail" {
			return a.Value.String()
		}
	}
	return err.Error()
}

func edit(old, replacement string) string {
	if !strings.Contains(minimal, old) {
		panic("harness: " + old + " is not in the minimal config")
	}
	return strings.Replace(minimal, old, replacement, 1)
}
