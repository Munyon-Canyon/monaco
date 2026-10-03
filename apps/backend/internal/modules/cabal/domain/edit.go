package domain

import (
	"log/slog"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type RulesPatch struct {
	JoinMode      *string
	VoterMode     *string
	Threshold     *string
	ExpirySeconds *int32
	SlippageBps   *int32
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
	return NewRules(join, voters, threshold, expiry, slippage)
}

func CanEdit(actor ids.UserID, cabal Cabal) error {
	const op = "cabal.CanEdit"
	switch {
	case !cabal.IsCreator(actor):
		return errs.New(errs.CodeNotCabalCreator, op)
	case cabal.Banned:
		return errs.New(errs.CodeCabalBanned, op)
	}
	return nil
}

func CheckVoters(cabal Cabal, members, voters []ids.UserID) error {
	const op = "cabal.CheckVoters"
	switch {
	case cabal.Rules.VoterMode() != VotersList:
		return errs.New(errs.CodeInvalidInput, op, slog.String("reason", "voter_ids_need_list_mode"))
	case !slices.Contains(voters, cabal.CreatorID):
		return errs.New(errs.CodeInvalidInput, op, slog.String("reason", "voter_ids_missing_creator"))
	}
	for _, voter := range voters {
		if !slices.Contains(members, voter) {
			return errs.New(errs.CodeInvalidInput, op, slog.String("reason", "voter_ids_not_member"))
		}
	}
	return nil
}
