package analytics_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func bannedKeys() []string {
	return []string{
		"email", "phone", "x_handle", "handle", "display_name", "wallet", "wallet_address", "address",
		"to_address", "signature", "tx_signature", "mint",
	}
}

func refusal(t *testing.T, c analytics.Capture) map[string]string {
	t.Helper()
	err := analytics.CheckNoPII(c)
	var coded *errs.Error
	if !errors.As(err, &coded) || coded.Code != errs.CodeAnalyticsPII {
		t.Fatalf("CheckNoPII(%+v) = %v, want analytics_pii", c, err)
	}
	attrs := map[string]string{}
	for _, a := range coded.Attrs {
		attrs[a.Key] = a.Value.String()
	}
	return attrs
}

func clean() analytics.Capture {
	return analytics.Capture{
		Event:      "probe_fired",
		DistinctID: "0190a5d0-0000-7000-8000-000000000001",
		Properties: map[string]any{
			"cabal_id": "0190a5d0-0000-7000-8000-000000000002", "symbol": "usdc", "first": true,
			"symbols": []any{"usdc", "aaplx"},
		},
		Set: map[string]any{"tier": "gold", "login_provider": "apple"},
	}
}

func keyOf(fill byte, n int) string { return string(chain.AddressOf(bytes.Repeat([]byte{fill}, n))) }

func key(n int) string { return keyOf(7, n) }

func TestAnalytics_CheckNoPII_acceptsACaptureMadeOfIDsFlagsAndNames(t *testing.T) {
	t.Parallel()
	if err := analytics.CheckNoPII(clean()); err != nil {
		t.Fatal(err)
	}
	if err := analytics.CheckNoPII(analytics.Capture{Event: "probe_fired", DistinctID: "system"}); err != nil {
		t.Fatalf("CheckNoPII with no properties = %v, want none", err)
	}
}

func TestAnalytics_CheckNoPII_acceptsValuesThatOnlyLookLikeIdentifiers(t *testing.T) {
	t.Parallel()
	lookalikes := map[string]string{
		"email without a dot":          "trader@localhost",
		"phone without a plus":         "14155550100",
		"phone too short":              "+123456",
		"phone too long":               "+1234567890123456",
		"phone starting with zero":     "+0415555010",
		"key one byte short":           key(31),
		"key one byte long":            key(33),
		"key with a character outside": strings.Repeat("0", 43),
		"hex id":                       "0190a5d000007000800000000000000a",
		"a sentence":                   "bought the dip at 5 usdc",
	}
	for name, value := range lookalikes {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := clean()
			c.Properties["value"] = value
			if err := analytics.CheckNoPII(c); err != nil {
				t.Fatalf("CheckNoPII with %q = %v, want none", value, err)
			}
		})
	}
}

func TestAnalytics_CheckNoPII_refusesEachBannedKeyWhereverItSits(t *testing.T) {
	t.Parallel()
	for _, banned := range bannedKeys() {
		t.Run(banned, func(t *testing.T) {
			t.Parallel()
			inProperties, inSet, nested, listed, shouted := clean(), clean(), clean(), clean(), clean()
			inProperties.Properties[banned] = "x"
			inSet.Set[banned] = "x"
			nested.Properties["cabal"] = map[string]any{"owner": map[string]any{banned: "x"}}
			listed.Properties["members"] = []any{"ok", map[string]any{banned: "x"}}
			shouted.Properties[strings.ToUpper(banned)] = "x"
			for name, tc := range map[string]struct {
				c    analytics.Capture
				path string
			}{
				"properties": {inProperties, "properties." + banned},
				"set":        {inSet, "set." + banned},
				"nested":     {nested, "properties.cabal.owner." + banned},
				"listed":     {listed, "properties.members[1]." + banned},
				"upper case": {shouted, "properties." + strings.ToUpper(banned)},
			} {
				got := refusal(t, tc.c)
				if got["path"] != tc.path || got["reason"] != "key" {
					t.Errorf("%s: attrs = %v, want path %s and reason key", name, got, tc.path)
				}
			}
		})
	}
}

func TestAnalytics_CheckNoPII_refusesEachValuePatternWhereverItSits(t *testing.T) {
	t.Parallel()
	longest := keyOf(0xff, 32)
	if len(longest) != 44 || len(chain.SystemProgram) != 32 {
		t.Fatalf("key fixtures have %d and %d characters, want the longest and the shortest, 44 and 32",
			len(longest), len(chain.SystemProgram))
	}
	patterns := map[string]struct{ value, reason string }{
		"email":          {"trader@example.com", "email"},
		"email in text":  {"reach me at trader@example.com today", "email"},
		"phone":          {"+14155550100", "phone"},
		"shortest phone": {"+1234567", "phone"},
		"longest phone":  {"+123456789012345", "phone"},
		"wallet key":     {key(32), "wallet_key"},
		"longest key":    {longest, "wallet_key"},
		"shortest key":   {string(chain.SystemProgram), "wallet_key"},
	}
	for name, p := range patterns {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			inProperties, inSet, nested, listed, distinct, event := clean(), clean(), clean(), clean(), clean(), clean()
			inProperties.Properties["note"] = p.value
			inSet.Set["note"] = p.value
			nested.Properties["cabal"] = map[string]any{"note": p.value}
			listed.Properties["notes"] = []any{"ok", p.value}
			distinct.DistinctID = p.value
			event.Event = p.value
			for where, tc := range map[string]struct {
				c    analytics.Capture
				path string
			}{
				"properties":  {inProperties, "properties.note"},
				"set":         {inSet, "set.note"},
				"nested":      {nested, "properties.cabal.note"},
				"listed":      {listed, "properties.notes[1]"},
				"distinct id": {distinct, "distinct_id"},
				"event":       {event, "event"},
			} {
				got := refusal(t, tc.c)
				if got["path"] != tc.path || got["reason"] != p.reason {
					t.Errorf("%s: attrs = %v, want path %s and reason %s", where, got, tc.path, p.reason)
				}
			}
		})
	}
}

func TestAnalytics_CheckNoPII_neverEchoesTheValueItRefuses(t *testing.T) {
	t.Parallel()
	const secret = "trader@example.com"
	c := clean()
	c.Properties["note"] = secret
	err := analytics.CheckNoPII(c)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("CheckNoPII error %v echoes the value it refused", err)
	}
	for _, v := range refusal(t, c) {
		if strings.Contains(v, secret) {
			t.Fatalf("error attr %q echoes the value it refused", v)
		}
	}
}

func TestAnalytics_CheckNoPII_refusesWhatItCannotInspect(t *testing.T) {
	t.Parallel()
	c := clean()
	c.Properties["bad"] = make(chan int)
	if got := refusal(t, c); got["reason"] != "unencodable" {
		t.Fatalf("attrs = %v, want reason unencodable", got)
	}
}
