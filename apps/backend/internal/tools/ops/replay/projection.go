package replay

import (
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

type Projector interface {
	Projection(source module.Set) []bus.HandlerSpec
}

func Projections(target, source module.Set) []bus.HandlerSpec {
	var out []bus.HandlerSpec
	for _, m := range target {
		if p, ok := m.(Projector); ok {
			out = append(out, p.Projection(source)...)
		}
	}
	return out
}
