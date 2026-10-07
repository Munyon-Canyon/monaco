package domain

type ReportKind string

const (
	ReportMessage ReportKind = "message"
	ReportComment ReportKind = "comment"
	ReportUser    ReportKind = "user"
	ReportCabal   ReportKind = "cabal"
)

func (k ReportKind) Valid() bool {
	switch k {
	case ReportMessage, ReportComment, ReportUser, ReportCabal:
		return true
	}
	return false
}

type ReportReason string

const (
	ReasonSpam  ReportReason = "spam"
	ReasonAbuse ReportReason = "abuse"
	ReasonOther ReportReason = "other"
)

func (r ReportReason) Valid() bool {
	switch r {
	case ReasonSpam, ReasonAbuse, ReasonOther:
		return true
	}
	return false
}

const MaxReportNote = 500
