package domain

import (
	"log/slog"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type JoinMode string

const (
	JoinOpen    JoinMode = "open"
	JoinRequest JoinMode = "request"
)

func (m JoinMode) valid() bool {
	switch m {
	case JoinOpen, JoinRequest:
		return true
	}
	return false
}

type VoterMode string

const (
	VotersAll  VoterMode = "all"
	VotersList VoterMode = "list"
)

func (m VoterMode) valid() bool {
	switch m {
	case VotersAll, VotersList:
		return true
	}
	return false
}

type Threshold string

const (
	ThresholdMajority  Threshold = "majority"
	ThresholdUnanimous Threshold = "unanimous"
)

func (t Threshold) valid() bool {
	switch t {
	case ThresholdMajority, ThresholdUnanimous:
		return true
	}
	return false
}

const (
	ExpiryHour int32 = 3600
	ExpiryDay  int32 = 86400
	ExpiryWeek int32 = 604800

	MinSlippageBps     int32 = 1
	MaxSlippageBps     int32 = 300
	DefaultSlippageBps int32 = 100

	MinMaxTradeBps  int32 = 100
	MaxMaxTradeBps  int32 = 10000
	MaxTradeBpsStep int32 = 100
)

func validExpiry(seconds int32) bool {
	switch seconds {
	case ExpiryHour, ExpiryDay, ExpiryWeek:
		return true
	}
	return false
}

type Rules struct {
	joinMode      JoinMode
	voterMode     VoterMode
	threshold     Threshold
	expirySeconds int32
	slippageBps   int32

	showSides          bool
	showSidesChangedAt *time.Time
	maxTradeBps        *int32
}

func NewRules(joinMode, voterMode, threshold string, expirySeconds, slippageBps int32) (Rules, error) {
	r := Rules{
		joinMode: JoinMode(joinMode), voterMode: VoterMode(voterMode), threshold: Threshold(threshold),
		expirySeconds: expirySeconds, slippageBps: slippageBps,
	}
	var bad []string
	for _, check := range []struct {
		field string
		ok    bool
	}{
		{"join_mode", r.joinMode.valid()},
		{"voter_mode", r.voterMode.valid()},
		{"threshold", r.threshold.valid()},
		{"proposal_expiry_seconds", validExpiry(r.expirySeconds)},
		{"slippage_bps", r.slippageBps >= MinSlippageBps && r.slippageBps <= MaxSlippageBps},
	} {
		if !check.ok {
			bad = append(bad, check.field)
		}
	}
	if len(bad) > 0 {
		return Rules{}, errs.New(errs.CodeInvalidInput, "cabal.NewRules", slog.String("fields", strings.Join(bad, ",")))
	}
	return r, nil
}

func (r Rules) JoinMode() JoinMode { return r.joinMode }

func (r Rules) VoterMode() VoterMode { return r.voterMode }

func (r Rules) Threshold() Threshold { return r.threshold }

func (r Rules) ExpirySeconds() int32 { return r.expirySeconds }

func (r Rules) SlippageBps() int32 { return r.slippageBps }

func (r Rules) ShowSides() bool { return r.showSides }

func (r Rules) ShowSidesChangedAt() *time.Time { return r.showSidesChangedAt }

func (r Rules) MaxTradeBps() *int32 { return r.maxTradeBps }

func (r Rules) WithShowSides(on bool, at *time.Time) Rules {
	r.showSides, r.showSidesChangedAt = on, at
	return r
}

func (r Rules) WithMaxTradeBps(v *int32) (Rules, error) {
	if v != nil && (*v < MinMaxTradeBps || *v > MaxMaxTradeBps || *v%MaxTradeBpsStep != 0) {
		return Rules{}, errs.New(errs.CodeInvalidInput, "cabal.WithMaxTradeBps", slog.String("fields", "max_trade_bps"))
	}
	r.maxTradeBps = v
	return r, nil
}

func (r Rules) WithVoterList() Rules {
	r.voterMode = VotersList
	return r
}
