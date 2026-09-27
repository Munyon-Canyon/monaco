package httpclient

import "time"

func WithFullDelay() Option {
	return func(c *Client) { c.jitter = func(d time.Duration) time.Duration { return d } }
}
