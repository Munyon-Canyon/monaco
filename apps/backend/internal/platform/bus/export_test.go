package bus

import "github.com/nats-io/nats.go"

func (c *Conn) NATS() *nats.Conn { return c.nc }

func (r *Registry) SetBeforeClosed(fn func()) { r.beforeClosed = fn }
