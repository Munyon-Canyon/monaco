package errs

const (
	CodeContentNotAllowed Code = "content_not_allowed"
	CodeReportNotFound    Code = "report_not_found"
)

func (codeFiles) SocialModeration() map[Code]Row {
	return map[Code]Row{
		CodeContentNotAllowed: {
			Name: "ContentNotAllowed", Kind: KindInvalid,
			Message: "That text uses a word Monaco does not allow. Please change it and try again.",
		},
		CodeReportNotFound: {
			Name: "ReportNotFound", Kind: KindNotFound, Message: "We could not find that report.",
		},
	}
}
