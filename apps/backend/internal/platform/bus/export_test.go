package bus

import (
	"time"

	"github.com/nats-io/nats.go"
)

func (c *Conn) NATS() *nats.Conn { return c.nc }

func WithCloseFlushTimeout(d time.Duration) Option {
	return func(o *options) { o.closeFlushTimeout = d }
}

func (r *Registry) SetBeforeClosed(fn func()) { r.beforeClosed = fn }
