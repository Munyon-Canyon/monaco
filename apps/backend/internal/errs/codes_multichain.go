package errs

const (
	CodeInvalidNetwork   Code = "invalid_network"
	CodeNotImplemented   Code = "not_implemented"
	CodeQuoteUnavailable Code = "quote_unavailable"
)

func (codeFiles) Multichain() map[Code]Row {
	return map[Code]Row{
		CodeInvalidNetwork: {Name: "InvalidNetwork", Kind: KindInvalid, Message: "That network is not supported."},
		CodeNotImplemented: {Name: "NotImplemented", Kind: KindUnavailable, Message: "This is not available yet."},
		CodeQuoteUnavailable: {
			Name: "QuoteUnavailable", Kind: KindUnavailable,
			Message: "We couldn't get a price for this stock. Try again in a moment.",
		},
	}
}
