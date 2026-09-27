package domain

type Account struct {
	ID      string
	Balance int64
}

const MaxAccounts = 3

func NewAccount(id string) Account {
	return Account{ID: id}
}

func (a Account) Credit(micros int64) Account {
	a.Balance += micros
	return a
}
