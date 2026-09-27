package clock

import "time"

type Ticker interface {
	Stop()
}

func NewTicker(d time.Duration) Ticker {
	return time.NewTicker(d)
}
