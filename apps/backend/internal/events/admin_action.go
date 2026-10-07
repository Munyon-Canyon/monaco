package events

import (
	"encoding/json"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	TypeAdminAction Type = "admin.action"

	minReasonRunes = 3
	maxReasonRunes = 500
)

type AdminActionKind string

const (
	AdminActionOpsPause       AdminActionKind = "ops_pause"
	AdminActionOpsResume      AdminActionKind = "ops_resume"
	AdminActionGlobalPause    AdminActionKind = "global_pause"
	AdminActionGlobalResume   AdminActionKind = "global_resume"
	AdminActionProposalVoid   AdminActionKind = "proposal_void"
	AdminActionUserBan        AdminActionKind = "user_ban"
	AdminActionUserUnban      AdminActionKind = "user_unban"
	AdminActionCommentRemove  AdminActionKind = "comment_remove"
	AdminActionHandleRevoke   AdminActionKind = "handle_revoke"
	AdminActionHandleReassign AdminActionKind = "handle_reassign"
	AdminActionCabalBan       AdminActionKind = "cabal_ban"
	AdminActionPingFlag       AdminActionKind = "ping_flag"
)

func (k AdminActionKind) Valid() bool {
	switch k {
	case AdminActionOpsPause, AdminActionOpsResume, AdminActionGlobalPause, AdminActionGlobalResume,
		AdminActionProposalVoid, AdminActionUserBan, AdminActionUserUnban, AdminActionCommentRemove,
		AdminActionHandleRevoke, AdminActionHandleReassign, AdminActionCabalBan, AdminActionPingFlag:
		return true
	}
	return false
}

type AdminTargetType string

const (
	AdminTargetCabal      AdminTargetType = "cabal"
	AdminTargetProposal   AdminTargetType = "proposal"
	AdminTargetUser       AdminTargetType = "user"
	AdminTargetComment    AdminTargetType = "comment"
	AdminTargetHandle     AdminTargetType = "handle"
	AdminTargetSystemPing AdminTargetType = "system_ping"
	AdminTargetGlobal     AdminTargetType = "global"
)

func (t AdminTargetType) Valid() bool {
	switch t {
	case AdminTargetCabal, AdminTargetProposal, AdminTargetUser, AdminTargetComment, AdminTargetHandle,
		AdminTargetSystemPing, AdminTargetGlobal:
		return true
	}
	return false
}

type Reason struct{ text string }

func NewReason(s string) (Reason, error) {
	text := strings.TrimSpace(s)
	if n := utf8.RuneCountInString(text); n < minReasonRunes || n > maxReasonRunes {
		return Reason{}, errs.New(errs.CodeReasonRequired, "events.NewReason")
	}
	return Reason{text: text}, nil
}

func (r Reason) String() string { return r.text }

type AdminAction struct {
	V          int             `json:"v"`
	ActionID   uuid.UUID       `json:"action_id"`
	AdminID    uuid.UUID       `json:"admin_id"    pii:"true"`
	Action     AdminActionKind `json:"action"`
	TargetType AdminTargetType `json:"target_type"`
	TargetID   string          `json:"target_id"   pii:"true"`
	Reason     string          `json:"reason"      pii:"true"`
	Before     json.RawMessage `json:"before"      pii:"false"`
	After      json.RawMessage `json:"after"       pii:"false"`
	ApprovedBy *uuid.UUID      `json:"approved_by" pii:"true"`
}

func (AdminAction) Type() Type { return TypeAdminAction }

func (AdminAction) AggregateType() string { return "admin_action" }

func (e AdminAction) AggregateID() uuid.UUID { return e.ActionID }

func NewAdminAction(
	id uuid.UUID, admin ids.UserID, action AdminActionKind, target AdminTargetType, targetID string,
	reason Reason, before, after any,
) (AdminAction, error) {
	const op = "events.NewAdminAction"
	if reason.String() == "" {
		return AdminAction{}, errs.New(errs.CodeReasonRequired, op)
	}
	if !action.Valid() || !target.Valid() || targetID == "" {
		return AdminAction{}, errs.New(errs.CodeInternal, op,
			slog.String("action", string(action)), slog.String("target_type", string(target)))
	}
	beforeJSON, err := json.Marshal(before)
	if err != nil {
		return AdminAction{}, errs.Wrap(err, errs.CodeInternal, op, slog.String("field", "before"))
	}
	afterJSON, err := json.Marshal(after)
	if err != nil {
		return AdminAction{}, errs.Wrap(err, errs.CodeInternal, op, slog.String("field", "after"))
	}
	return AdminAction{
		V: 1, ActionID: id, AdminID: admin.UUID(), Action: action, TargetType: target, TargetID: targetID,
		Reason: reason.String(), Before: beforeJSON, After: afterJSON,
	}, nil
}
