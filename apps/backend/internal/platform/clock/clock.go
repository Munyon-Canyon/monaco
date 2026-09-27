package clock

import "time"

type Clock interface {
	Now() time.Time
	NewTicker(d time.Duration) Ticker
	After(d time.Duration) <-chan time.Time
}

type Ticker interface {
	C() <-chan time.Time
	Stop()
}

type Real struct{}

func (Real) Now() time.Time { return time.Now() }

func (Real) NewTicker(d time.Duration) Ticker { return realTicker{t: time.NewTicker(d)} }

func (Real) After(d time.Duration) <-chan time.Time { return time.After(d) }

type realTicker struct{ t *time.Ticker }

func (r realTicker) C() <-chan time.Time { return r.t.C }

func (r realTicker) Stop() { r.t.Stop() }
