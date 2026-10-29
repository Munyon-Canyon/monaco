package domain_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
)

func baseRules(t *testing.T) domain.Rules {
	t.Helper()
	rules, err := domain.NewRules("open", "all", "majority", domain.ExpiryDay, domain.DefaultSlippageBps)
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

func TestNewRules_startsPrivateWithNoCap(t *testing.T) {
	t.Parallel()
	rules := baseRules(t)
	if rules.ShowSides() || rules.ShowSidesChangedAt() != nil || rules.MaxTradeBps() != nil {
		t.Fatalf("rules = sides %v at %v cap %v, want off, never changed, no cap",
			rules.ShowSides(), rules.ShowSidesChangedAt(), rules.MaxTradeBps())
	}
}

func ptr(v int32) *int32 { return &v }

func TestRules_WithMaxTradeBpsTakesNoneOrAWholePercentFromOneToOneHundred(t *testing.T) {
	t.Parallel()
	for _, in := range []*int32{nil, ptr(100), ptr(2000), ptr(10000)} {
		got, err := baseRules(t).WithMaxTradeBps(in)
		if err != nil || (in == nil) != (got.MaxTradeBps() == nil) || in != nil && *got.MaxTradeBps() != *in {
			t.Fatalf("WithMaxTradeBps(%v) = %v, %v", in, got.MaxTradeBps(), err)
		}
	}
}

func TestRules_WithMaxTradeBpsRefusesAnythingElse(t *testing.T) {
	t.Parallel()
	for _, in := range []int32{0, 99, 150, 10100, -100} {
		got, err := baseRules(t).WithMaxTradeBps(ptr(in))
		if errs.CodeOf(err) != errs.CodeInvalidInput || got.MaxTradeBps() != nil {
			t.Fatalf("WithMaxTradeBps(%d) = %v, %v; want %s", in, got.MaxTradeBps(), err, errs.CodeInvalidInput)
		}
	}
}

func TestRules_WithShowSidesLeavesTheOriginalAlone(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, time.October, 20, 12, 0, 0, 0, time.UTC)
	base := baseRules(t)
	on := base.WithShowSides(true, &at)
	if !on.ShowSides() || on.ShowSidesChangedAt() == nil || !on.ShowSidesChangedAt().Equal(at) {
		t.Fatalf("on = %v at %v, want on at %v", on.ShowSides(), on.ShowSidesChangedAt(), at)
	}
	if base.ShowSides() || base.ShowSidesChangedAt() != nil {
		t.Fatal("WithShowSides changed the receiver")
	}
}

func TestRules_PatchKeepsTheSidesAndCapRules(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, time.October, 20, 12, 0, 0, 0, time.UTC)
	limit := int32(2000)
	rules, err := baseRules(t).WithShowSides(true, &at).WithMaxTradeBps(&limit)
	if err != nil {
		t.Fatal(err)
	}
	slippage := int32(50)
	patched, err := rules.Patch(domain.RulesPatch{SlippageBps: &slippage})
	if err != nil {
		t.Fatal(err)
	}
	if patched.SlippageBps() != 50 || !patched.ShowSides() || !patched.ShowSidesChangedAt().Equal(at) ||
		patched.MaxTradeBps() == nil || *patched.MaxTradeBps() != 2000 {
		t.Fatalf("patched = %+v, want slippage 50 with sides on since %v and cap 2000", patched, at)
	}
}
