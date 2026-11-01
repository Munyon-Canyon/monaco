package domain

import (
	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type RulesPatch struct {
	JoinMode      *string
	VoterMode     *string
	Threshold     *string
	ExpirySeconds *int32
	SlippageBps   *int32
}

func (p RulesPatch) ChangesRules() bool {
	return p.JoinMode != nil || p.VoterMode != nil || p.Threshold != nil || p.ExpirySeconds != nil
}

func (r Rules) Patch(p RulesPatch) (Rules, error) {
	join, voters, threshold := string(r.joinMode), string(r.voterMode), string(r.threshold)
	expiry, slippage := r.expirySeconds, r.slippageBps
	if p.JoinMode != nil {
		join = *p.JoinMode
	}
	if p.VoterMode != nil {
		voters = *p.VoterMode
	}
	if p.Threshold != nil {
		threshold = *p.Threshold
	}
	if p.ExpirySeconds != nil {
		expiry = *p.ExpirySeconds
	}
	if p.SlippageBps != nil {
		slippage = *p.SlippageBps
	}
	next, err := NewRules(join, voters, threshold, expiry, slippage)
	if err != nil {
		return Rules{}, err
	}
	next.showSides, next.showSidesChangedAt, next.maxTradeBps = r.showSides, r.showSidesChangedAt, r.maxTradeBps
	return next, nil
}

func CanEdit(actor Actor, cabal Cabal) error {
	const op = "cabal.CanEdit"
	switch {
	case !actor.Member || !cabal.IsCreator(actor.UserID):
		return errs.New(errs.CodeNotCabalCreator, op)
	case cabal.Banned:
		return errs.New(errs.CodeCabalBanned, op)
	}
	return nil
}
