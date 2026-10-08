package errs

const (
	CodeAlreadyPaused Code = "already_paused"
	CodeNoOpsPause    Code = "no_ops_pause"
)

func (codeFiles) FundingOps() map[Code]Row {
	return map[Code]Row{
		CodeAlreadyPaused: {
			Name: "AlreadyPaused", Kind: KindConflict,
			Message: "Trading is already paused here by an operator.",
		},
		CodeNoOpsPause: {
			Name: "NoOpsPause", Kind: KindConflict,
			Message: "No operator pause is open here, so there is nothing to resume.",
		},
	}
}
