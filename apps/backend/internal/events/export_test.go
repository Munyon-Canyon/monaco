package events

import "reflect"

func GoTypes() map[Type]reflect.Type {
	regs := registrations()
	types := make(map[Type]reflect.Type, len(regs))
	for _, r := range regs {
		types[r.typ] = r.goType
	}
	return types
}
