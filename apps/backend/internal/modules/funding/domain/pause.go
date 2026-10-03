package domain

type PauseReason string

const (
	PauseReasonExternalDeposit PauseReason = "external_deposit"
	PauseReasonOps             PauseReason = "ops"
)
