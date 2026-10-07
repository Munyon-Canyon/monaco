package errs

const CodeTxnNotFound Code = "txn_not_found"

func (codeFiles) AdminLookups() map[Code]Row {
	return map[Code]Row{
		CodeTxnNotFound: {Name: "TxnNotFound", Kind: KindNotFound, Message: "We could not find that transaction."},
	}
}
