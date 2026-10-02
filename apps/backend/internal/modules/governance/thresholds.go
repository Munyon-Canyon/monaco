package governance

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type cabalThresholds struct {
	cabals cabal.Queries
}

func (c cabalThresholds) Threshold(ctx context.Context, id ids.CabalID) (domain.ThresholdRule, error) {
	rules, err := c.cabals.Rules(ctx, id)
	if err != nil {
		return "", err
	}
	return domain.ParseThresholdRule(string(rules.Threshold))
}
