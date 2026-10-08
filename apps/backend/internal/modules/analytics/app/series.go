package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
)

type EventCounter interface {
	CountEvents(ctx context.Context, query bus.CountQuery) ([]bus.EventCount, error)
}

type SeriesRead struct {
	Metric  string
	Type    string
	GroupBy string
	Present string
}

type SeriesPoint struct {
	Start  time.Time
	Metric string
	Group  string
	Count  int64
}

func ReadSeries(
	ctx context.Context, counter EventCounter, w Window, reads []SeriesRead,
) ([]SeriesPoint, error) {
	var out []SeriesPoint
	for _, read := range reads {
		rows, err := counter.CountEvents(ctx, bus.CountQuery{
			Types: []string{read.Type}, From: w.From, To: w.To, Size: w.Size, GroupBy: read.GroupBy,
			Present: read.Present,
		})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			out = append(out, SeriesPoint{Start: row.Start, Metric: read.Metric, Group: row.Group, Count: row.Count})
		}
	}
	return out, nil
}
