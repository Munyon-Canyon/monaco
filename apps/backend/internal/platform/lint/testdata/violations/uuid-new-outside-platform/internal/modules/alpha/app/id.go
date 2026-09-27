package app

import "github.com/google/uuid"

func ID() uuid.UUID {
	return uuid.New()
}
