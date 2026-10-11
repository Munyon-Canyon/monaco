package errs

const (
	CodeInvalidNetwork Code = "invalid_network"
	CodeNotImplemented Code = "not_implemented"
)

func (codeFiles) Multichain() map[Code]Row {
	return map[Code]Row{
		CodeInvalidNetwork: {Name: "InvalidNetwork", Kind: KindInvalid, Message: "That network is not supported."},
		CodeNotImplemented: {Name: "NotImplemented", Kind: KindUnavailable, Message: "This is not available yet."},
	}
}
