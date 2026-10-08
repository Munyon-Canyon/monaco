package port

import "context"

type Counts struct {
	Cabals     int64
	Banned     int64
	MembersP50 int64
	MembersP90 int64
	MembersMax int64
}

type Dashboard interface {
	Counts(ctx context.Context) (Counts, error)
}
