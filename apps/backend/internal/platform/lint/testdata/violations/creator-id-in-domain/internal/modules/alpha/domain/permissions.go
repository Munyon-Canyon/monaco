package domain

type Cabal struct {
	CreatorID string
}

func (c Cabal) IsCreator(user string) bool {
	return c.CreatorID == user
}
