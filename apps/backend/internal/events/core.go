package events

type Core interface {
	Type() Type
	core()
}
