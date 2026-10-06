package feed

import (
	"encoding/json"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type ProposalStatus string

const (
	StatusOpen             ProposalStatus = "open"
	StatusPassed           ProposalStatus = "passed"
	StatusExecutionFailed  ProposalStatus = "execution_failed"
	StatusFailed           ProposalStatus = "failed"
	StatusExpired          ProposalStatus = "expired"
	StatusWithdrawn        ProposalStatus = "withdrawn"
	StatusVoided           ProposalStatus = "voided"
	StatusExecuted         ProposalStatus = "executed"
	StatusExecutionBlocked ProposalStatus = "execution_blocked"
)

func proposalStatuses() []ProposalStatus {
	return []ProposalStatus{
		StatusOpen, StatusPassed, StatusExecutionFailed, StatusFailed, StatusExpired, StatusWithdrawn,
		StatusVoided, StatusExecuted, StatusExecutionBlocked,
	}
}

func (s ProposalStatus) rank() int {
	switch s {
	case StatusOpen:
		return 1
	case StatusPassed, StatusExecutionFailed:
		return 2
	case StatusFailed, StatusExpired, StatusWithdrawn, StatusVoided, StatusExecuted, StatusExecutionBlocked:
		return 3
	}
	return 3
}

func (s ProposalStatus) MovesFrom() []string {
	var from []string
	for _, c := range proposalStatuses() {
		if c.rank() < s.rank() || (c == StatusPassed && s == StatusExecutionFailed) {
			from = append(from, string(c))
		}
	}
	return from
}

func statusLabel(status, code string) string {
	switch ProposalStatus(status) {
	case StatusOpen:
		return "Open"
	case StatusPassed:
		return "Passed"
	case StatusExecutionFailed:
		return "Trade failed"
	case StatusFailed:
		return "Failed"
	case StatusExpired:
		return "Expired"
	case StatusWithdrawn:
		return "Withdrawn"
	case StatusVoided:
		return "Voided"
	case StatusExecuted:
		return "Executed"
	case StatusExecutionBlocked:
		return "Blocked: " + errs.Message(errs.Code(code))
	default:
		return ""
	}
}

func StatusPatch(s ProposalStatus, code string) []byte {
	raw, _ := json.Marshal(struct {
		Status     ProposalStatus `json:"status"`
		StatusCode string         `json:"status_code"`
	}{s, code})
	return raw
}
