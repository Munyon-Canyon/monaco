package app

type Row struct {
	CreatorID string
}

func CanEdit(row Row, user string) bool {
	return row.CreatorID == user
}
