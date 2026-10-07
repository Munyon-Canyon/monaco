package errs

const CodeDeadLetterNotOpen Code = "dead_letter_not_open"

func (codeFiles) AdminDeadLetters() map[Code]Row {
	return map[Code]Row{
		CodeDeadLetterNotOpen: {
			Name: "DeadLetterNotOpen", Kind: KindConflict, Message: "This dead letter was already resolved.",
		},
	}
}
