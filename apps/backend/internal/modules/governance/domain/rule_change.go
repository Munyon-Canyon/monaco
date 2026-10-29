package domain

import (
	"bytes"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const ruleChangeVersion = 1

type Rule string

const (
	RuleShowSides   Rule = "show_sides"
	RuleMaxTradeBps Rule = "max_trade_bps"
	RuleThreshold   Rule = "threshold"
	RuleExpiry      Rule = "proposal_expiry_seconds"
	RuleJoinMode    Rule = "join_mode"
	RuleVoter       Rule = "voter"
	RuleAdmin       Rule = "admin"
)

var (
	ErrRuleUnchanged  = ruleCheck("unchanged")
	ErrLastAdmin      = ruleCheck("last_admin")
	ErrAdminNotVoter  = ruleCheck("admin_not_voter")
	ErrTargetsCreator = ruleCheck("targets_creator")
	ErrNotMemberRule  = ruleCheck("no_member")
)

func ruleCheck(reason string) error {
	return errs.New(errs.CodeInvalidInput, "governance.RuleChange", slog.String("reason", reason))
}

const (
	maxTradeBpsMin  int32 = 100
	maxTradeBpsMax  int32 = 10000
	maxTradeBpsStep int32 = 100
)

type RuleChange struct {
	rule Rule
	from any
	to   any
	user *ids.UserID
}

func ShowSidesChange(from, to bool) RuleChange {
	return RuleChange{rule: RuleShowSides, from: from, to: to}
}

func MaxTradeBpsChange(from, to *int32) (RuleChange, error) {
	for _, v := range []*int32{from, to} {
		if v != nil && (*v < maxTradeBpsMin || *v > maxTradeBpsMax || *v%maxTradeBpsStep != 0) {
			return RuleChange{}, invalidRule(RuleMaxTradeBps)
		}
	}
	return RuleChange{rule: RuleMaxTradeBps, from: optional(from), to: optional(to)}, nil
}

func ThresholdChange(from, to string) (RuleChange, error) {
	if !validThreshold(from) || !validThreshold(to) {
		return RuleChange{}, invalidRule(RuleThreshold)
	}
	return RuleChange{rule: RuleThreshold, from: from, to: to}, nil
}

func ExpiryChange(from, to int32) (RuleChange, error) {
	if !validExpiry(from) || !validExpiry(to) {
		return RuleChange{}, invalidRule(RuleExpiry)
	}
	return RuleChange{rule: RuleExpiry, from: from, to: to}, nil
}

func JoinModeChange(from, to string) (RuleChange, error) {
	if !validJoinMode(from) || !validJoinMode(to) {
		return RuleChange{}, invalidRule(RuleJoinMode)
	}
	return RuleChange{rule: RuleJoinMode, from: from, to: to}, nil
}

func VoterChange(user ids.UserID, from, to bool) RuleChange {
	return RuleChange{rule: RuleVoter, from: from, to: to, user: &user}
}

func AdminChange(user ids.UserID, from, to bool) RuleChange {
	return RuleChange{rule: RuleAdmin, from: from, to: to, user: &user}
}

func optional(v *int32) any {
	if v == nil {
		return nil
	}
	return *v
}

func validThreshold(v string) bool { return v == "majority" || v == "unanimous" }

func validJoinMode(v string) bool { return v == "open" || v == "request" }

func validExpiry(v int32) bool { return v == 3600 || v == 86400 || v == 604800 }

func invalidRule(rule Rule) error {
	return errs.New(errs.CodeInvalidInput, "governance.RuleChange", slog.String("field", string(rule)))
}

func (c RuleChange) Rule() Rule { return c.rule }

func (c RuleChange) From() any { return c.from }

func (c RuleChange) To() any { return c.to }

func (c RuleChange) UserID() *ids.UserID { return c.user }

func (c RuleChange) Unchanged() error {
	if c.from == c.to {
		return ErrRuleUnchanged
	}
	return nil
}

func (c RuleChange) KeepsAnAdmin(admins int) error {
	if c.rule == RuleAdmin && c.to == false && admins <= 1 {
		return ErrLastAdmin
	}
	return nil
}

func (c RuleChange) AdminIsVoter(isVoter bool) error {
	if c.rule == RuleAdmin && c.to == true && !isVoter {
		return ErrAdminNotVoter
	}
	return nil
}

func (c RuleChange) NotCreator(creator ids.UserID) error {
	if c.user != nil && *c.user == creator {
		return ErrTargetsCreator
	}
	return nil
}

type ruleChangeWire struct {
	V      int             `json:"v"`
	Rule   Rule            `json:"rule"`
	From   json.RawMessage `json:"from"`
	To     json.RawMessage `json:"to"`
	UserID *uuid.UUID      `json:"user_id,omitempty"`
}

type ruleChangeOut struct {
	V      int        `json:"v"`
	Rule   Rule       `json:"rule"`
	From   any        `json:"from"`
	To     any        `json:"to"`
	UserID *uuid.UUID `json:"user_id,omitempty"`
}

func (c RuleChange) MarshalJSON() ([]byte, error) {
	out := ruleChangeOut{V: ruleChangeVersion, Rule: c.rule, From: c.from, To: c.to}
	if c.user != nil {
		id := c.user.UUID()
		out.UserID = &id
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "governance.RuleChange")
	}
	return raw, nil
}

func (c *RuleChange) UnmarshalJSON(raw []byte) error {
	var wire ruleChangeWire
	if err := json.Unmarshal(raw, &wire); err != nil {
		return errs.Wrap(err, errs.CodeDecodeFailed, "governance.RuleChange")
	}
	if wire.V != ruleChangeVersion {
		return errs.New(errs.CodeDecodeFailed, "governance.RuleChange", slog.Int("v", wire.V))
	}
	out, err := decodeRule(wire)
	if err != nil {
		return err
	}
	*c = out
	return nil
}

func decodeRule(w ruleChangeWire) (RuleChange, error) {
	switch w.Rule {
	case RuleShowSides, RuleVoter, RuleAdmin:
		return decodeBoolRule(w)
	case RuleMaxTradeBps:
		var from, to *int32
		if err := decodeBoth(w, &from, &to); err != nil {
			return RuleChange{}, err
		}
		return MaxTradeBpsChange(from, to)
	case RuleExpiry:
		var from, to int32
		if err := decodeBoth(w, &from, &to); err != nil {
			return RuleChange{}, err
		}
		return ExpiryChange(from, to)
	case RuleThreshold, RuleJoinMode:
		var from, to string
		if err := decodeBoth(w, &from, &to); err != nil {
			return RuleChange{}, err
		}
		if w.Rule == RuleThreshold {
			return ThresholdChange(from, to)
		}
		return JoinModeChange(from, to)
	}
	return RuleChange{}, unknown("governance.RuleChange", string(w.Rule))
}

func decodeBoolRule(w ruleChangeWire) (RuleChange, error) {
	var from, to bool
	if err := decodeBoth(w, &from, &to); err != nil {
		return RuleChange{}, err
	}
	switch {
	case w.Rule == RuleShowSides:
		return ShowSidesChange(from, to), nil
	case w.UserID == nil:
		return RuleChange{}, ErrNotMemberRule
	case w.Rule == RuleVoter:
		return VoterChange(ids.UserIDFrom(*w.UserID), from, to), nil
	}
	return AdminChange(ids.UserIDFrom(*w.UserID), from, to), nil
}

func decodeBoth[T any](w ruleChangeWire, from, to *T) error {
	for _, field := range []struct {
		raw json.RawMessage
		dst *T
	}{{w.From, from}, {w.To, to}} {
		dec := json.NewDecoder(bytes.NewReader(field.raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(field.dst); err != nil {
			return errs.Wrap(err, errs.CodeDecodeFailed, "governance.RuleChange")
		}
	}
	return nil
}
