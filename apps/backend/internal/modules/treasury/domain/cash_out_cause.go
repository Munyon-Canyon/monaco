package domain

type CashOutCause string

const (
	CashOutByMember CashOutCause = "member"
	CashOutWindDown CashOutCause = "wind_down"
)

func (c CashOutCause) OrMember() CashOutCause {
	if c == "" {
		return CashOutByMember
	}
	return c
}
