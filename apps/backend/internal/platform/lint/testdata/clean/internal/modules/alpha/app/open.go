package app

import "github.com/monaco/monaco/apps/backend/internal/modules/alpha/domain"

func Open(id string) domain.Account {
	return domain.NewAccount(id).Credit(1)
}
