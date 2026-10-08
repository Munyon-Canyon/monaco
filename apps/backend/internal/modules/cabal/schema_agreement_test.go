package cabal_test

import (
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestSchemaAndDomainAgreeOnEveryValueOfTheStringRules(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	for _, tt := range []struct {
		column string
		values []string
		with   func(*rulesArgs, string)
	}{
		{
			"join_mode",
			[]string{string(domain.JoinOpen), string(domain.JoinRequest), "", "invite_only", "Open"},
			func(a *rulesArgs, v string) { a.join = v },
		},
		{
			"voter_mode",
			[]string{string(domain.VotersAll), string(domain.VotersList), "", "some", "ALL"},
			func(a *rulesArgs, v string) { a.voters = v },
		},
		{
			"threshold",
			[]string{string(domain.ThresholdMajority), string(domain.ThresholdUnanimous), "", "plurality"},
			func(a *rulesArgs, v string) { a.threshold = v },
		},
	} {
		for _, v := range tt.values {
			args := baseRules()
			tt.with(&args, v)
			_, domainErr := args.build()
			schemaErr := run(t, pool, `UPDATE cabals SET `+tt.column+` = $1 WHERE id = $2`, v, c.ID.UUID())
			if (domainErr == nil) != (schemaErr == nil) {
				t.Errorf("%s = %q: the domain says %v and the schema says %v", tt.column, v, domainErr, schemaErr)
			}
		}
	}
}

func TestSchemaAndDomainAgreeOnEveryValueOfTheNumericRules(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	for _, tt := range []struct {
		column string
		values []int32
		with   func(*rulesArgs, int32)
	}{
		{"proposal_expiry_seconds", []int32{
			0, -1, domain.ExpiryHour - 1, domain.ExpiryHour, domain.ExpiryHour + 1, domain.ExpiryDay, domain.ExpiryDay + 1,
			domain.ExpiryWeek, domain.ExpiryWeek + 1,
		}, func(a *rulesArgs, v int32) { a.expiry = v }},
		{"slippage_bps", []int32{
			-1, 0, domain.MinSlippageBps, domain.DefaultSlippageBps, domain.MaxSlippageBps, domain.MaxSlippageBps + 1,
		}, func(a *rulesArgs, v int32) { a.bps = v }},
	} {
		for _, v := range tt.values {
			args := baseRules()
			tt.with(&args, v)
			_, domainErr := args.build()
			schemaErr := run(t, pool, `UPDATE cabals SET `+tt.column+` = $1 WHERE id = $2`, v, c.ID.UUID())
			if (domainErr == nil) != (schemaErr == nil) {
				t.Errorf("%s = %d: the domain says %v and the schema says %v", tt.column, v, domainErr, schemaErr)
			}
		}
	}
}

func TestSchemaAndDomainAgreeOnTheNameLength(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	for _, unit := range []string{"x", "é", "☕"} {
		for _, n := range []int{0, domain.MinNameLen - 1, domain.MinNameLen, domain.MaxNameLen, domain.MaxNameLen + 1} {
			name := strings.Repeat(unit, n)
			_, domainErr := domain.ParseName(name)
			schemaErr := run(t, pool, `UPDATE cabals SET name = $1 WHERE id = $2`, name, c.ID.UUID())
			if (domainErr == nil) != (schemaErr == nil) {
				t.Errorf("%d x %q: the domain says %v and the schema says %v", n, unit, domainErr, schemaErr)
			}
		}
	}
}
