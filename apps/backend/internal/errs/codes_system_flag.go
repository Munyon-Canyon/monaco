package errs

const CodeAlreadyFlagged Code = "already_flagged"

func (codeFiles) SystemFlag() map[Code]Row {
	return map[Code]Row{
		CodeAlreadyFlagged: {Name: "AlreadyFlagged", Kind: KindConflict, Message: "This ping is already flagged."},
	}
}
